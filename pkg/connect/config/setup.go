package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	connct "go.minekube.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"go.minekube.com/gate/pkg/connect"
	"go.minekube.com/gate/pkg/connect/single"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/process"
	"go.minekube.com/gate/pkg/util/netutil"
)

type Instance interface {
	proxy.ServerRegistry
	ConnHandler
}

type ConnHandler interface {
	HandleConn(conn net.Conn)
}

// New validates the config and creates a process collection from it.
func New(c Config, log logr.Logger, inst Instance) (process.Collection, error) {
	c.Transport = strings.ToLower(c.Transport)
	switch c.Transport {
	case "":
		c.Transport = "ws" // default to WebSocket
	case "ws", "grpc":
	default:
		return nil, fmt.Errorf("unsupported transport %q: choose ws or grpc", c.Transport)
	}

	coll := process.New(process.Options{AllOrNothing: true})

	if c.Enabled {
		if err := addConnectClient(coll, c, log.WithName("client").WithName("watch"), inst); err != nil {
			return nil, err
		}
	}
	if c.Service.Enabled {
		if c.Service.PublicTunnelServiceAddr == "" {
			c.Service.PublicTunnelServiceAddr = c.Service.Addr
		}
		ctx, cancel := context.WithTimeout(context.TODO(), timeout)
		defer cancel()
		if err := addService(ctx, coll, c, log.WithName("service"), inst); err != nil {
			return nil, err
		}
	}

	return coll, nil
}

const timeout = time.Minute

func addConnectClient(coll process.Collection, c Config, log logr.Logger, connHandler ConnHandler) error {
	if c.Name == "" {
		return errors.New("missing name for our endpoint")
	}
	if c.WatchServiceAddr == "" {
		return errors.New("missing watch service address for listening to session proposals")
	}

	var dialOpts []grpc.DialOption
	if c.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Register ourselves and watch for sessions
	return addRetried(coll, log, func(ctx context.Context) error {
		log.Info("Connecting to watch service...",
			"addr", c.WatchServiceAddr, "timeout", timeout.String())
		// dialCtx, cancel := context.WithTimeout(ctx, timeout)
		// defer cancel()
		t := time.Now()
		// cli, cc, err := connect.DialWatchService(dialCtx, c.WatchServiceAddr, dialOpts...)
		// if err != nil {
		// 	return err
		// }
		// defer cc.Close()

		log.Info("Connected", "took", time.Since(t).String())
		err := connect.Watch(ctx, connect.WatchOptions{
			Name: c.Name,
			// Client:            cli,
			ConnHandler:       func(conn connect.TunnelConn) { connHandler.HandleConn(conn) },
			TunnelDialOptions: dialOpts,
			Log:               log,
		})
		if ctx.Err() == nil {
			// Reconnect to WatchService
			if err == nil {
				// TODO Backoff reconnect without logging an error after 5 times
				err = errors.New("disconnected by watch service")
			}
		}
		fmt.Println("watch service closed after", time.Since(t))
		return err
	})
}

func addService(ctx context.Context, coll process.Collection, c Config, log logr.Logger, reg proxy.ServerRegistry) error {
	if c.Service.PublicTunnelServiceAddr == "" {
		c.Service.PublicTunnelServiceAddr = c.Service.Addr
	}

	// Listener for Connect services
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", c.Service.Addr)
	if err != nil {
		return fmt.Errorf("could not setup listener on %q for Connect services: %w", c.Service.Addr, err)
	}

	acceptor, err := single.New(single.Options{
		Log:                     log,
		ServerRegistry:          reg,
		PublicTunnelServiceAddr: c.Service.PublicTunnelServiceAddr,
		OverrideRegistration:    c.Service.OverrideRegistration,
	})
	if err != nil {
		return fmt.Errorf("error creating single-instance Connect: %w", err)
	}

	return coll.Add(process.RunnableFunc(func(stop <-chan struct{}) error {
		defer ln.Close()
		defer log.Info("Stopped serving Connect services")

		// svr := grpc.NewServer()

		ws := &connct.WatchService{
			StartWatch: connect.AcceptEndpoint(acceptor),
		}
		ts := &connct.TunnelService{
			AcceptTunnel: connect.AcceptInboundTunnel(acceptor),
			LocalAddr:    ln.Addr(),
		}
		svr := http.Server{
			Addr: c.Service.Addr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Println(r.URL, r.RequestURI, r.UserAgent(),
					r.Header, r.RemoteAddr, r.Trailer, r.Host, r.TLS)
				r = r.WithContext(peer.NewContext(r.Context(), &peer.Peer{Addr: netutil.NewAddr(r.RemoteAddr, "tcp")}))
				md := metadata.MD{}
				r = r.WithContext(metadata.NewIncomingContext(r.Context(), md))
				if strings.Contains(r.RequestURI, "watch") {
					md.Set(connct.MDEndpoint, r.Header.Get(connct.MDEndpoint))
					fmt.Println("handle watch")
					ws.ServeHTTP(w, r)
					fmt.Println("handled watch")
					return
				}
				md.Set(connct.MDSession, r.Header.Get(connct.MDSession))
				fmt.Println("handle tunnel")
				ts.ServeHTTP(w, r)
				fmt.Println("handled tunnel")
			}),
		}

		log.Info("Serving Connect services")
		go func() { <-stop; _ = svr.Close() }()
		return svr.Serve(ln)
	}))
}

func addRetried(coll process.Collection, log logr.Logger, fn func(ctx context.Context) error, afterFns ...func()) error {
	return coll.Add(retrying(log, ctxStop(fn), afterFns...))
}

func retrying(log logr.Logger, r process.Runnable, afterFns ...func()) process.Runnable {
	log = log.WithName("retry")
	const after = time.Second * 5
	return process.RunnableFunc(func(stop <-chan struct{}) error {
		defer func() {
			for _, fn := range afterFns {
				fn()
			}
		}()

		var err error
		for {
			if err = r.Start(stop); err != nil {
				select {
				case <-stop:
					return err
				default:
					log.Info("Error while running process, retrying...",
						"error", err, "retryAfter", after.String())
					sleep(stop, after)
					continue // retry
				}
			}
			return nil
		}
	})
}

func ctxStop(r func(ctx context.Context) error) process.Runnable {
	return process.RunnableFunc(func(stop <-chan struct{}) error {
		ctx, cancel := chanToCtx(stop)
		defer cancel()
		return r(ctx)
	})
}

func chanToCtx(stop <-chan struct{}) (ctx context.Context, cancel context.CancelFunc) {
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		defer cancel()
		select {
		case <-stop:
		case <-ctx.Done():
		}
	}()
	return
}

func sleep(stop <-chan struct{}, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-stop:
	}
}

package config

import (
	"context"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.minekube.com/gate/pkg/connect"
	"go.minekube.com/gate/pkg/connect/embedded"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/process"
)

// New validates the config and creates a process collection from it.
func New(c Config, log logr.Logger, connHandler ConnHandler) (process.Collection, error) {
	coll := process.New(process.Options{
		Logger:       log,
		AllOrNothing: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if c.Enabled {
		if err := addConnectClient(ctx, coll, c, log, connHandler); err != nil {
			return nil, err
		}
	}

	if c.Services.Watch.Enabled {

	}

	return coll, nil
}

// ConnHandler can handle connections.
type ConnHandler interface {
	HandleConn(conn net.Conn)
	embedded.ServerRegistry
}

func addConnectClient(ctx context.Context, coll process.Collection, c Config, log logr.Logger, connHandler ConnHandler) error {
	if c.Name == "" {
		return errors.New("missing name for our endpoint")
	}
	if c.WatchServiceAddr == "" {
		return errors.New("missing watch service address for listening to session proposals")
	}

	dialOpts := []grpc.DialOption{grpc.WithBlock()}
	if c.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	log.Info("Connecting to watch service", "addr", c.WatchServiceAddr)
	cli, cc, err := connect.DialWatchService(ctx, c.WatchServiceAddr, dialOpts...)
	if err != nil {
		return err
	}

	// Register ourselves and watch for sessions
	addRetried(coll, log, func(ctx context.Context) error {
		return connect.Watch(ctx, connect.WatchOptions{
			Name:              c.Name,
			Cli:               cli,
			ConnHandler:       func(conn connect.TunnelConn) { connHandler.HandleConn(conn) },
			TunnelDialOptions: dialOpts,
			Log:               log.WithName("watch"),
		})
	}, func() {
		// Close client connection afterwards
		_ = cc.Close()
	})
}

func addRetried(coll process.Collection, log logr.Logger, fn func(ctx context.Context) error, afterFns ...func()) {
	_ = coll.Add(retrying(log, ctxStop(fn), afterFns...))
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
						"error", err, "retryAfter", after)
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

package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/go-logr/logr"
	"go.minekube.com/connect"
	"go.minekube.com/connect/ws"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/process"
	"go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/connectutil/single"
)

func service(c Config, reg proxy.ServerRegistry) (process.Runnable, error) {
	if c.Service.PublicTunnelServiceAddr == "" {
		c.Service.PublicTunnelServiceAddr = c.Service.Addr
	}

	ln, err := single.New(single.Options{
		ServerRegistry:          reg,
		PublicTunnelServiceAddr: c.Service.PublicTunnelServiceAddr,
		OverrideRegistration:    c.Service.OverrideRegistration,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating single-instance Connect: %w", err)
	}

	opts := ws.ServerOptions{}
	mux := http.NewServeMux()
	mux.Handle("/watch", requireHeader(
		[]string{connect.MDEndpoint},
		opts.EndpointHandler(connectutil.RequireEndpointName(ln)),
	))
	mux.Handle("/tunnel", requireHeader(
		[]string{connect.MDSession},
		opts.TunnelHandler(connectutil.RequireTunnelSessionID(ln)),
	))

	return process.RunnableFunc(func(ctx context.Context) error {
		svr := http.Server{
			Addr:    c.Service.Addr,
			Handler: mux,
			ConnContext: func(ctx context.Context, c net.Conn) context.Context {
				return connect.WithTunnelOptions(ctx, connect.TunnelOptions{
					LocalAddr:  c.LocalAddr(),
					RemoteAddr: c.RemoteAddr(),
				})
			},
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() { <-ctx.Done(); _ = svr.Close() }()

		log := logr.FromContextOrDiscard(ctx)
		log.Info("Connect service started", "addr", c.Service.Addr)
		defer log.Info("Stopped Connect service")

		err = svr.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return err
	}), nil
}

func requireHeader(header []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range header {
			name := r.Header.Get(h)
			if name == "" {
				err := fmt.Sprintf("missing request %s header", h)
				http.Error(w, err, http.StatusBadRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

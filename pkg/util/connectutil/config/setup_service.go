package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"go.minekube.com/connect"
	"go.minekube.com/connect/ws"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/process"
	"go.minekube.com/gate/pkg/util/connectutil/single"
)

func service(c Config, log logr.Logger, reg proxy.ServerRegistry) (process.Runnable, error) {
	if c.Service.PublicTunnelServiceAddr == "" {
		c.Service.PublicTunnelServiceAddr = c.Service.Addr
	}

	acceptor, err := single.New(single.Options{
		Log:                     log,
		ServerRegistry:          reg,
		PublicTunnelServiceAddr: c.Service.PublicTunnelServiceAddr,
		OverrideRegistration:    c.Service.OverrideRegistration,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating single-instance Connect: %w", err)
	}

	opts := ws.ServerOptions{}
	e := opts.EndpointHandler()
	t := opts.TunnelHandler()

	mux := http.NewServeMux()
	mux.Handle("/tunnel", e)
	mux.Handle("/watch", t)

	return ctxRunnable(func(ctx context.Context) error {
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
		err = svr.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return err
	}), nil
}

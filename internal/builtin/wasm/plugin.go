package wasm

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type managerLifecycle interface {
	Start(context.Context, *proxy.Proxy) error
	Close() error
}

func Plugin(cfg config.Wasm) proxy.Plugin {
	return plugin(New(cfg))
}

func plugin(manager managerLifecycle) proxy.Plugin {
	return proxy.Plugin{
		Name: "wasm",
		Init: func(ctx context.Context, gateProxy *proxy.Proxy) error {
			if err := manager.Start(ctx, gateProxy); err != nil {
				return err
			}
			log := logr.FromContextOrDiscard(ctx).WithName("wasm")
			event.Subscribe(gateProxy.Event(), 0, func(*proxy.ShutdownEvent) {
				if err := manager.Close(); err != nil {
					log.Error(err, "error closing wasm plugins")
				}
			})
			return nil
		},
	}
}

package proxy

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
)

func componentManagerPlugin(manager ComponentPluginManager) Plugin {
	return Plugin{
		Name: "component",
		Init: func(ctx context.Context, gateProxy *Proxy) error {
			if err := manager.Start(ctx, gateProxy); err != nil {
				return err
			}
			log := logr.FromContextOrDiscard(ctx)
			event.Subscribe(gateProxy.Event(), 0, func(*ShutdownEvent) {
				if err := manager.Close(); err != nil {
					log.Error(err, "error closing component plugins")
				}
			})
			return nil
		},
	}
}

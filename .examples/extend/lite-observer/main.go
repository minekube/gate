package main

import (
	"context"
	"fmt"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/cmd/gate"
	"go.minekube.com/gate/pkg/edition/java/lite"
)

func main() {
	lite.Plugins = append(lite.Plugins, lite.Plugin{
		Name: "LiteObserver",
		Init: func(ctx context.Context, rt *lite.Runtime) error {
			event.Subscribe(rt.Event(), 0, func(e *lite.ForwardStartedEvent) {
				active := rt.ActiveForwardsByClientIP(e.ClientIP)
				fmt.Printf("lite forward started id=%s client=%s backend=%s host=%s route=%s active_for_ip=%d\n",
					e.ConnectionID, e.ClientIP, e.BackendAddr, e.Host, e.RouteID, len(active))
			})
			event.Subscribe(rt.Event(), 0, func(e *lite.ForwardEndedEvent) {
				fmt.Printf("lite forward ended id=%s reason=%s host=%s route=%s\n",
					e.ConnectionID, e.Reason, e.Host, e.RouteID)
			})
			return nil
		},
	})

	gate.Execute()
}

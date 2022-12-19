package developers

import (
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func SubscribeExample(p *proxy.Proxy) {
	// Get the event manager.
	mgr := p.Event()

	// Subscribe to an event.
	const priority = 0
	event.Subscribe(mgr, priority, func(e *proxy.PreLoginEvent) {
		// Kicks every player
		e.Deny(&component.Text{Content: "Sorry, the server is in maintenance."})
	})
}

// Package reload provides configuration reloading capabilities.
package reload

import (
	"github.com/robinbraemer/event"
)

// ConfigUpdateEvent is fired when the config is reloaded.
type ConfigUpdateEvent[T any] struct {
	// Config is the new config.
	Config *T
}

// Event implements event.Event.
var _ event.Event = (*ConfigUpdateEvent[any])(nil)

// Subscribe subscribes the given handler to the config update event.
func Subscribe[T any](mgr event.Manager, handler func(*ConfigUpdateEvent[T])) func() {
	return event.Subscribe(mgr, 0, handler)
}

// Map maps the config update event to another config type.
func Map[C1, C2 event.Event](mgr event.Manager, forwarder func(*C1) *C2) func() {
	if mgr.HasSubscriber(&ConfigUpdateEvent[C1]{}) {
		return func() {}
	}
	return event.Subscribe(mgr, 0, func(e *ConfigUpdateEvent[C1]) {
		c2 := forwarder(e.Config)
		mgr.Fire(&ConfigUpdateEvent[C2]{Config: c2})
	})
}

// FireConfigUpdate fires the config update event.
func FireConfigUpdate[T any](mgr event.Manager, config *T) {
	mgr.Fire(&ConfigUpdateEvent[T]{Config: config})
}

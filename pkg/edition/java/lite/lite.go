package lite

import "github.com/robinbraemer/event"

// Lite encapsulates all lite mode functionality for a Gate proxy instance.
// This provides a clean abstraction for lite mode features and avoids global state.
type Lite struct {
	strategyManager *StrategyManager
	runtime         *Runtime
}

// NewLite creates a new Lite instance for a Gate proxy.
func NewLite() *Lite {
	return NewLiteWithEvent(event.Nop)
}

// NewLiteWithEvent creates a new Lite instance for a Gate proxy with the provided event manager.
func NewLiteWithEvent(mgr event.Manager) *Lite {
	rt := newRuntime(mgr)
	sm := NewStrategyManager()
	sm.setRuntime(rt)
	return &Lite{
		strategyManager: sm,
		runtime:         rt,
	}
}

// StrategyManager returns the strategy manager for load balancing.
func (l *Lite) StrategyManager() *StrategyManager {
	return l.strategyManager
}

// Runtime returns the Lite runtime for events and read-only observability state.
func (l *Lite) Runtime() *Runtime {
	return l.runtime
}

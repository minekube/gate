package lite

import (
	"net/netip"
	"sync/atomic"

	"github.com/robinbraemer/event"
)

// Runtime provides read-only Lite observability facilities for extensions.
type Runtime struct {
	event   event.Manager
	tracker *activeForwardTracker

	observabilityEnabled atomic.Bool
}

func newRuntime(mgr event.Manager) *Runtime {
	if mgr == nil {
		mgr = event.Nop
	}
	return &Runtime{
		event:   mgr,
		tracker: newActiveForwardTracker(),
	}
}

// Event returns the Lite event manager.
func (r *Runtime) Event() event.Manager {
	if r == nil || r.event == nil {
		return event.Nop
	}
	return r.event
}

// ActiveForwardsByClientIP returns active forward snapshots for a client IP.
func (r *Runtime) ActiveForwardsByClientIP(ip netip.Addr) []ActiveForward {
	if r == nil {
		return nil
	}
	return r.tracker.listByClientIP(ip)
}

// ActiveForward returns an active forward snapshot by connection ID.
func (r *Runtime) ActiveForward(id string) (ActiveForward, bool) {
	if r == nil {
		return ActiveForward{}, false
	}
	return r.tracker.get(id)
}

func (r *Runtime) maybeEnableObservability() bool {
	if r == nil {
		return false
	}
	if r.observabilityEnabled.Load() {
		return true
	}
	if len(Plugins) != 0 ||
		r.Event().HasSubscriber((*ForwardStartedEvent)(nil)) ||
		r.Event().HasSubscriber((*ForwardEndedEvent)(nil)) {
		r.observabilityEnabled.Store(true)
		return true
	}
	return false
}

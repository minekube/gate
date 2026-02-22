package lite

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"go.minekube.com/gate/pkg/util/netutil"
)

// ActiveForward is a read-only snapshot of a currently active Lite TCP forward.
type ActiveForward struct {
	ConnectionID string
	ClientIP     netip.Addr
	BackendAddr  net.Addr
	Host         string
	RouteID      string
	StartedAt    time.Time
}

type activeForwardTracker struct {
	nextID atomic.Uint64

	mu sync.RWMutex

	byID       map[string]ActiveForward
	byClientIP map[netip.Addr]map[string]struct{}
}

func newActiveForwardTracker() *activeForwardTracker {
	return &activeForwardTracker{
		byID:       make(map[string]ActiveForward),
		byClientIP: make(map[netip.Addr]map[string]struct{}),
	}
}

func (t *activeForwardTracker) nextConnectionID() string {
	id := t.nextID.Add(1)
	return fmt.Sprintf("lfwd-%d", id)
}

func (t *activeForwardTracker) add(f ActiveForward) {
	f = cloneActiveForward(f)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.byID[f.ConnectionID] = f
	if !f.ClientIP.IsValid() {
		return
	}
	idx := t.byClientIP[f.ClientIP]
	if idx == nil {
		idx = make(map[string]struct{})
		t.byClientIP[f.ClientIP] = idx
	}
	idx[f.ConnectionID] = struct{}{}
}

func (t *activeForwardTracker) remove(id string) (ActiveForward, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	f, ok := t.byID[id]
	if !ok {
		return ActiveForward{}, false
	}
	delete(t.byID, id)
	if f.ClientIP.IsValid() {
		if idx := t.byClientIP[f.ClientIP]; idx != nil {
			delete(idx, id)
			if len(idx) == 0 {
				delete(t.byClientIP, f.ClientIP)
			}
		}
	}
	return cloneActiveForward(f), true
}

func (t *activeForwardTracker) get(id string) (ActiveForward, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	f, ok := t.byID[id]
	if !ok {
		return ActiveForward{}, false
	}
	return cloneActiveForward(f), true
}

func (t *activeForwardTracker) listByClientIP(ip netip.Addr) []ActiveForward {
	t.mu.RLock()
	defer t.mu.RUnlock()

	idx := t.byClientIP[ip]
	if len(idx) == 0 {
		return nil
	}

	out := make([]ActiveForward, 0, len(idx))
	for id := range idx {
		if f, ok := t.byID[id]; ok {
			out = append(out, cloneActiveForward(f))
		}
	}
	return out
}

func cloneActiveForward(f ActiveForward) ActiveForward {
	f.BackendAddr = cloneAddr(f.BackendAddr)
	return f
}

func cloneAddr(addr net.Addr) net.Addr {
	if addr == nil {
		return nil
	}
	return netutil.NewAddr(addr.String(), addr.Network())
}

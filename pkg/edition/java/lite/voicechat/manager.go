package voicechat

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"slices"
	"sync"

	"github.com/go-logr/logr"
)

// Manager owns one Relay per distinct listen address and keeps that set in
// sync with the Lite configuration.
type Manager struct {
	mu     sync.Mutex
	ctx    context.Context // nil until Start
	log    logr.Logger
	relays map[string]*managedRelay
}

type managedRelay struct {
	relay    *Relay
	sessions *Sessions
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewManager returns a manager that does nothing until Start is called.
func NewManager() *Manager {
	return &Manager{relays: map[string]*managedRelay{}}
}

// Start binds the given listen addresses. Relays stop when ctx is done.
func (m *Manager) Start(ctx context.Context, log logr.Logger, listens []string) error {
	m.mu.Lock()
	m.ctx, m.log = ctx, log
	m.mu.Unlock()
	return m.Reconcile(listens)
}

// Reconcile makes the running listeners match listens. Before Start it does
// nothing. If any new address fails to bind, nothing is changed.
func (m *Manager) Reconcile(listens []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return nil
	}
	want := map[string]bool{}
	for _, l := range listens {
		want[l] = true
	}

	// Bind everything new first so a failure leaves the running state intact.
	bound := map[string]net.PacketConn{}
	for l := range want {
		if _, ok := m.relays[l]; ok {
			continue
		}
		pc, err := net.ListenPacket("udp", l)
		if err != nil {
			for _, p := range bound {
				_ = p.Close()
			}
			return fmt.Errorf("voice chat: listen on %s: %w", l, err)
		}
		bound[l] = pc
	}

	for l, mr := range m.relays {
		if !want[l] {
			mr.cancel()
			<-mr.done
			delete(m.relays, l)
			m.log.Info("voice chat relay stopped", "listen", l)
		}
	}
	for l, pc := range bound {
		s := NewSessions()
		r := NewRelay(pc, s, RelayOptions{}, m.log.WithValues("listen", l))
		ctx, cancel := context.WithCancel(m.ctx)
		mr := &managedRelay{relay: r, sessions: s, cancel: cancel, done: make(chan struct{})}
		go func() { defer close(mr.done); _ = r.Serve(ctx) }()
		m.relays[l] = mr
		m.log.Info("voice chat relay listening", "listen", l)
	}
	return nil
}

// Register records a TCP session of ip for the relay bound to listen.
// It is a no-op (with a no-op release) if no relay listens there.
func (m *Manager) Register(ip netip.Addr, listen, voiceAddr string) (release func()) {
	m.mu.Lock()
	mr := m.relays[listen]
	m.mu.Unlock()
	if mr == nil {
		return func() {}
	}
	return mr.sessions.Register(ip, voiceAddr)
}

// Listening returns the sorted listen addresses of running relays.
func (m *Manager) Listening() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Sorted(maps.Keys(m.relays))
}

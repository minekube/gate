// Package voicechat relays Simple Voice Chat UDP traffic for Gate Lite.
//
// The relay is a trust boundary: it only forwards datagrams from client IPs
// that currently hold a Lite TCP session, otherwise any host could use Gate as
// an open UDP relay towards the backends.
package voicechat

import (
	"maps"
	"net/netip"
	"slices"
	"sync"
)

// Sessions tracks which voice backends a client IP is currently connected to.
type Sessions struct {
	mu   sync.Mutex
	byIP map[netip.Addr]map[string]int
}

// NewSessions returns an empty table.
func NewSessions() *Sessions {
	return &Sessions{byIP: map[netip.Addr]map[string]int{}}
}

// Register records that ip is connected to the voice backend address (UDP host:port).
// The returned release function is idempotent.
func (s *Sessions) Register(ip netip.Addr, backend string) (release func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byIP[ip]
	if m == nil {
		m = map[string]int{}
		s.byIP[ip] = m
	}
	m[backend]++

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			m := s.byIP[ip]
			if m == nil {
				return
			}
			m[backend]--
			if m[backend] <= 0 {
				delete(m, backend)
			}
			if len(m) == 0 {
				delete(s.byIP, ip)
			}
		})
	}
}

// Backends returns the sorted voice backend addresses registered for ip.
func (s *Sessions) Backends(ip netip.Addr) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byIP[ip]
	if len(m) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(m))
}

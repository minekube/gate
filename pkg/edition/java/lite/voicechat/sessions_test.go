package voicechat

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionsRegisterAndRelease(t *testing.T) {
	s := NewSessions()
	ip := netip.MustParseAddr("203.0.113.5")
	require.Empty(t, s.Backends(ip))

	r1 := s.Register(ip, "b:1")
	r2 := s.Register(ip, "a:1")
	r3 := s.Register(ip, "a:1") // second player, same backend, same IP
	require.Equal(t, []string{"a:1", "b:1"}, s.Backends(ip))

	r2()
	require.Equal(t, []string{"a:1", "b:1"}, s.Backends(ip), "refcounted: one session still holds a:1")
	r3()
	require.Equal(t, []string{"b:1"}, s.Backends(ip))
	r3() // idempotent: must not underflow or drop b:1
	require.Equal(t, []string{"b:1"}, s.Backends(ip))
	r1()
	require.Empty(t, s.Backends(ip))
}

func TestSessionsIsolatedPerIP(t *testing.T) {
	s := NewSessions()
	a, b := netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")
	rel := s.Register(a, "x:1")
	require.Empty(t, s.Backends(b))
	rel()
}

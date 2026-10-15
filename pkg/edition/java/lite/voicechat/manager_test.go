package voicechat

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = pc.Close() }()
	return pc.LocalAddr().String()
}

func TestManagerReconcileBeforeStartIsNoop(t *testing.T) {
	m := NewManager()
	require.NoError(t, m.Reconcile([]string{"127.0.0.1:0"}))
	require.Empty(t, m.Listening())
}

func TestManagerStartReconcileAndStop(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := freeUDPAddr(t), freeUDPAddr(t)

	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))
	require.Equal(t, []string{a}, m.Listening())

	require.NoError(t, m.Reconcile([]string{a, b}))
	require.ElementsMatch(t, []string{a, b}, m.Listening())

	// Removing a route's voiceChat closes its listener (port is free again).
	require.NoError(t, m.Reconcile([]string{b}))
	require.Equal(t, []string{b}, m.Listening())
	pc, err := net.ListenPacket("udp", a)
	require.NoError(t, err, "removed listener must release its port")
	_ = pc.Close()

	cancel()
	require.Eventually(t, func() bool {
		pc, err := net.ListenPacket("udp", b)
		if err != nil {
			return false
		}
		_ = pc.Close()
		return true
	}, 2*time.Second, 20*time.Millisecond, "ctx cancel must release all listeners")
}

// A failing bind rejects the whole reconcile and keeps what runs.
func TestManagerBindFailureLeavesStateUnchanged(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := freeUDPAddr(t)
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))

	busy, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = busy.Close() }()

	err = m.Reconcile([]string{busy.LocalAddr().String()}) // would also remove a
	require.Error(t, err)
	require.Equal(t, []string{a}, m.Listening(), "a must keep running")

	// Rollback also closes sockets bound earlier in the same attempt.
	fresh := freeUDPAddr(t)
	require.Error(t, m.Reconcile([]string{a, fresh, busy.LocalAddr().String()}))
	require.Equal(t, []string{a}, m.Listening())
	pc, err := net.ListenPacket("udp", fresh)
	require.NoError(t, err, "socket bound during a failed reconcile must be closed")
	_ = pc.Close()
}

func TestManagerRegisterRoutesToRelaySessions(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := freeUDPAddr(t)
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))
	be := newFakeBackend(t, 9)

	release := m.Register(loopback, a, be.addr())
	c, err := net.Dial("udp", a)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	_, _ = c.Write(voicePacket(9))
	require.Equal(t, voicePacket(9), be.recv(t))

	release()
	// unknown listen: harmless no-op
	m.Register(netip.MustParseAddr("192.0.2.1"), "127.0.0.1:1", "x:1")()
}

func TestManagerStartTwiceFails(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, m.Start(ctx, logr.Discard(), nil))
	require.Error(t, m.Start(ctx, logr.Discard(), nil))
}

func TestManagerReconcileAfterCancelIsNoop(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, m.Start(ctx, logr.Discard(), nil))
	cancel()
	require.NoError(t, m.Reconcile([]string{freeUDPAddr(t)}))
	require.Empty(t, m.Listening())
}

func TestManagerDropsDeadRelayAndRebinds(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := freeUDPAddr(t)
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))

	m.mu.Lock()
	conn := m.relays[a].relay.conn
	m.mu.Unlock()
	require.NoError(t, conn.Close()) // socket dies while ctx is live

	require.Eventually(t, func() bool { return len(m.Listening()) == 0 },
		2*time.Second, 10*time.Millisecond, "dead relay must be dropped")
	require.NoError(t, m.Reconcile([]string{a}))
	require.Equal(t, []string{a}, m.Listening())
}

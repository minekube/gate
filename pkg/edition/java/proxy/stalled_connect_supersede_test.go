package proxy

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/util/uuid"
)

// wedgedTunnelConn is a backend connection whose writes never complete and
// whose Close does not reach them: the shape of a Connect tunnel stream whose
// peer stopped draining, where the reset is dropped instead of failing the
// write. A player moved onto such a server is parked: the write never returns,
// so the connect never returns, and nothing in the session ever runs the
// teardown that would unregister it.
type wedgedTunnelConn struct {
	release   chan struct{}
	closeOnce sync.Once
	writes    chan struct{}
	local     net.Addr
	remote    net.Addr
}

func newWedgedTunnelConn() *wedgedTunnelConn {
	return &wedgedTunnelConn{
		release: make(chan struct{}),
		writes:  make(chan struct{}, 64),
		local:   &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1},
		remote:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2},
	}
}

func (c *wedgedTunnelConn) Read([]byte) (int, error) { <-c.release; return 0, io.EOF }
func (c *wedgedTunnelConn) Write([]byte) (int, error) {
	select {
	case c.writes <- struct{}{}:
	default:
	}
	<-c.release // never completes while the test runs
	return 0, io.ErrClosedPipe
}

// Close reports success without failing the blocked write, like a reset that is
// dropped on a stream nobody is draining.
func (c *wedgedTunnelConn) Close() error {
	c.closeOnce.Do(func() {})
	return nil
}

// unblock is test cleanup only.
func (c *wedgedTunnelConn) unblock() {
	c.closeOnce.Do(func() {})
	select {
	case <-c.release:
	default:
		close(c.release)
	}
}

func (c *wedgedTunnelConn) LocalAddr() net.Addr              { return c.local }
func (c *wedgedTunnelConn) RemoteAddr() net.Addr             { return c.remote }
func (c *wedgedTunnelConn) SetDeadline(time.Time) error      { return nil }
func (c *wedgedTunnelConn) SetReadDeadline(time.Time) error  { return nil }
func (c *wedgedTunnelConn) SetWriteDeadline(time.Time) error { return nil }

type wedgedDialerServerInfo struct {
	ServerInfo
	conn net.Conn
}

func (d *wedgedDialerServerInfo) Dial(context.Context, Player) (net.Conn, error) {
	return d.conn, nil
}

// A kicked player whose redirect target never completes is parked with no
// server and a move in flight; its client connection is gone, so nothing in the
// session will ever finish that move. The identity must not stay held: after the
// budget that bounds every move, the reconnect is accepted instead of being
// refused with "You are already connected to this server!" for as long as the
// machine lives.
func TestReconnectSupersedesRegistrationStalledInServerConnect(t *testing.T) {
	const connectTimeout = 300 * time.Millisecond
	p, hub, logs := newKickWedgeProxy(t, connectTimeout, "stall")
	defer hub.close()

	wedged := newWedgedTunnelConn()
	t.Cleanup(wedged.unblock)
	_, err := p.Register(&wedgedDialerServerInfo{
		ServerInfo: NewServerInfo("stall", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3}),
		conn:       wedged,
	})
	require.NoError(t, err)

	player, _ := placedPlayer(t, p, "Alson", uuid.New(), 30*time.Second)
	go kickPlayerFromEndpoint(p, player)

	// Wait until the session is parked: no current server, move in flight.
	require.Eventually(t, func() bool {
		return player.CurrentServer() == nil && player.connectionInFlight() != nil
	}, 3*time.Second, 10*time.Millisecond, "the kick must leave the player mid-move\nproxy log:\n%s",
		joinLines(logs.snapshot()))

	reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Alson", player.ID())
	deadline := time.Now().Add(2 * time.Second)
	for !p.canRegisterConnection(reconnect) {
		if time.Now().After(deadline) {
			t.Fatalf("reconnect refused 2s after the move outlived its %s connect budget: the "+
				"parked registration holds the identity\nproxy log:\n%s",
				2*connectTimeout, joinLines(logs.snapshot()))
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.True(t, p.registerConnection(reconnect))
	require.Equal(t, Player(reconnect), p.PlayerByName("Alson"))
}

// The stall rule must be narrow: a move that is still within its budget is a
// move that may still arrive, so the duplicate login is refused as before.
func TestReconnectRefusedWhileServerConnectIsWithinBudget(t *testing.T) {
	p, hub, _ := newKickWedgeProxy(t, 10*time.Second, "stall")
	defer hub.close()

	wedged := newWedgedTunnelConn()
	t.Cleanup(wedged.unblock)
	_, err := p.Register(&wedgedDialerServerInfo{
		ServerInfo: NewServerInfo("stall", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3}),
		conn:       wedged,
	})
	require.NoError(t, err)

	player, _ := placedPlayer(t, p, "KelsonGame", uuid.New(), 30*time.Second)
	go kickPlayerFromEndpoint(p, player)
	require.Eventually(t, func() bool {
		return player.CurrentServer() == nil && player.connectionInFlight() != nil
	}, 3*time.Second, 10*time.Millisecond)

	duplicate := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "KelsonGame", player.ID())
	require.False(t, p.canRegisterConnection(duplicate),
		"a move still inside its connect budget must not be superseded")
	require.False(t, p.registerConnection(duplicate))
}

// A player that is genuinely on a server keeps its identity even while a move
// is in flight - a server switch is not a stale session.
func TestPlayingSessionWithMoveInFlightIsNotSuperseded(t *testing.T) {
	p, hub, _ := newKickWedgeProxy(t, 10*time.Second, "stall")
	defer hub.close()

	player, _ := placedPlayer(t, p, "Symtra_", uuid.New(), 30*time.Second)
	player.setInFlightConnection(newServerConnection(p.Server("smp").(*registeredServer), nil, player))
	player.mu.Lock()
	player.connInFlightStart = time.Now().Add(-time.Hour) // long past any budget
	player.mu.Unlock()

	duplicate := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Symtra_", player.ID())
	require.False(t, p.canRegisterConnection(duplicate),
		"a placed session with a current server must never be superseded")
}

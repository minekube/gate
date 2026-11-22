package proxy

import (
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Connect-edge production shape (2026-11-22, edge machine 185d2d6a7e93e8):
// five players were kicked from an endpoint at 13:59:44 when their session
// tunnel closed. moxy redirected each of them to the browser hub, the hub
// session was accepted and then "timed out before player reached the session
// server" ~30 s later, and for three of the five no disconnect line was ever
// logged again: every reconnect was refused with "You are already connected to
// this server!" for 10+ minutes, until the machine was replaced.
//
// The connection Gate is handed on that edge is moxy's tunnel streamConn: its
// SetWriteDeadline (and SetReadDeadline) return nil without arming anything,
// because the libp2p stream underneath has no deadline support. So once the
// client stops reading - which is what a player quitting after a failed
// redirect does - the farewell write of the kick blocks forever, and because
// the close only runs after that write the teardown never unregisters the
// player. The registration then looks like a live PLAY session to
// supersedableRegistration and every later login is refused.
//
// These tests pin both halves of the fix: the session is released (reaped) when
// the write budget is spent even though the write never returns, and a
// reconnect is accepted immediately instead of waiting for the reaper.

// vanishedClientConn models that connection: reads and writes never complete
// because the peer is gone, the deadline setters are no-ops, and Close still
// runs the player's teardown - which is netmc's contract and the only thing
// that releases the registration.
type vanishedClientConn struct {
	*duplicateLoginTestConn

	writeBudget time.Duration
	closed      chan struct{}
	closeOnce   sync.Once
	teardown    func()
	writes      atomic.Int32
}

func newVanishedClientConn(writeBudget time.Duration) *vanishedClientConn {
	return &vanishedClientConn{
		duplicateLoginTestConn: newDuplicateLoginTestConn(state.Play),
		writeBudget:            writeBudget,
		closed:                 make(chan struct{}),
	}
}

// WriteTimeout is the configured per-flush budget a real netmc connection
// reports; netmc.CloseWith bounds the farewell write with it.
func (c *vanishedClientConn) WriteTimeout() time.Duration { return c.writeBudget }

func (c *vanishedClientConn) WritePacket(proto.Packet) error {
	c.writes.Add(1)
	<-c.closed // the peer never reads: only our own Close unblocks the write
	return netmc.ErrClosedConn
}

func (c *vanishedClientConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.duplicateLoginTestConn.Close()
		close(c.closed)
		if c.teardown != nil {
			c.teardown()
		}
	})
	return nil
}

// newKickWedgeProxy boots a real Gate with an endpoint whose redirect target
// ("hub") accepts connections and never answers, which is how a hub session
// that never completes presents itself to Gate.
func newKickWedgeProxy(t *testing.T, connectionTimeout time.Duration, redirectTo string) (*Proxy, *blackHoleBackend, *logCapture) {
	t.Helper()
	hub := newBlackHoleBackend(t)

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ConnectionTimeout = configutil.Duration(connectionTimeout)
	cfg.Servers = map[string]string{
		"smp": "127.0.0.1:25565",
		"hub": hub.addr(),
	}
	cfg.Try = nil
	cfg.FailoverOnUnexpectedServerDisconnect = true

	p, err := New(Options{Config: &cfg, EventMgr: event.New()})
	require.NoError(t, err)
	logs := &logCapture{}
	p.log = funcr.New(logs.log, funcr.Options{Verbosity: 1})
	require.NoError(t, p.init())

	// moxy's KickedFromServerEvent listener redirects the kicked player to the
	// browser hub endpoint - once. If that hand-over fails too, the player is
	// disconnected, which is the "player was kicked from server {Internal server
	// connection error}" the edge logged for each of the five.
	var redirected atomic.Bool
	event.Subscribe(p.Event(), 0, func(e *KickedFromServerEvent) {
		if _, ok := e.Result().(*DisconnectPlayerKickResult); ok && !redirected.Swap(true) {
			e.SetResult(&RedirectPlayerKickResult{Server: p.Server(redirectTo)})
		}
	})
	return p, hub, logs
}

// kickPlayerFromEndpoint fires a backend PLAY connection loss for a placed
// player exactly as backendPlaySessionHandler.Disconnected does.
func kickPlayerFromEndpoint(p *Proxy, player *connectedPlayer) {
	player.handleDisconnectWithReason(p.Server("smp"), internalServerConnectionError, true)
}

// placedPlayer registers a player that already reached PLAY on "smp", with a
// client connection that has stopped reading.
func placedPlayer(t *testing.T, p *Proxy, name string, id uuid.UUID, writeBudget time.Duration) (*connectedPlayer, *vanishedClientConn) {
	t.Helper()
	conn := newVanishedClientConn(writeBudget)
	player := newDuplicateLoginTestPlayer(p, conn, name, id)
	player.virtualHost = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565}
	player.log = p.log.WithValues("player", name)
	conn.teardown = player.teardown
	player.setConnectedServer(newServerConnection(p.Server("smp").(*registeredServer), nil, player))
	require.True(t, p.registerConnection(player))
	return player, conn
}

// A kick whose farewell write never returns must still release the
// registration: the close, and with it the teardown, is enforced by Gate's own
// write budget instead of depending on the peer reading. Without this the ghost
// session survives until the process restarts.
func TestKickWithVanishedClientReleasesRegistration(t *testing.T) {
	const writeBudget = 300 * time.Millisecond
	p, hub, logs := newKickWedgeProxy(t, 200*time.Millisecond, "hub")
	defer hub.close()

	player, conn := placedPlayer(t, p, "Alson", uuid.New(), writeBudget)
	require.Equal(t, 1, p.PlayerCount())

	t.Logf("before kick: active=%v closed=%v state=%v", player.Active(), netmc.Closed(conn), player.State())
	kicked := make(chan struct{})
	go func() {
		defer close(kicked)
		kickPlayerFromEndpoint(p, player)
	}()

	waitForPlayerCountIn(t, p, 0, writeBudget+3*time.Second, logs)
	require.Nil(t, p.PlayerByName("Alson"), "the wedged registration must be released")
	require.True(t, netmc.Closed(conn), "the client connection must be closed by the reaper")

	select {
	case <-kicked:
	case <-time.After(5 * time.Second):
		t.Fatal("the kick path never returned")
	}
}

// A reconnect after such a kick must be accepted immediately, not after the
// reaper and certainly not never. This is the player-visible half: the same 20
// sessions are kicked while their redirect target hangs, and all 20 reconnects
// must be accepted within 2 s each.
func TestReconnectAfterKickWithVanishedClientIsAccepted(t *testing.T) {
	const (
		players = 20
		// Past the 2 s reconnect window, so a pass cannot be the reaper's doing:
		// the reconnect is accepted because the wedged registration is
		// superseded, not because it was cleaned up in time.
		writeBudget = 3 * time.Second
	)
	p, hub, logs := newKickWedgeProxy(t, 200*time.Millisecond, "hub")
	defer hub.close()

	type session struct {
		player *connectedPlayer
		conn   *vanishedClientConn
		id     uuid.UUID
	}
	sessions := make([]session, 0, players)
	for i := 0; i < players; i++ {
		name := "Wedged" + string(rune('A'+i))
		id := uuid.New()
		player, conn := placedPlayer(t, p, name, id, writeBudget)
		sessions = append(sessions, session{player: player, conn: conn, id: id})
	}

	kicks := make([]chan struct{}, 0, players)
	for _, s := range sessions {
		done := make(chan struct{})
		go func(player *connectedPlayer) {
			defer close(done)
			kickPlayerFromEndpoint(p, player)
		}(s.player)
		kicks = append(kicks, done)
	}

	for i, s := range sessions {
		deadline := time.Now().Add(2 * time.Second)
		reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), s.player.Username(), s.id)
		for !p.canRegisterConnection(reconnect) {
			if time.Now().After(deadline) {
				t.Fatalf("reconnect for %s was still refused 2s after the kick: the wedged "+
					"registration is not released (%d/%d accepted)\nproxy log:\n%s",
					s.player.Username(), i, players, strings.Join(logs.snapshot(), "\n"))
			}
			time.Sleep(5 * time.Millisecond)
		}
		require.True(t, p.registerConnection(reconnect),
			"the reconnect of %s must take over the wedged registration", s.player.Username())
		require.Equal(t, Player(reconnect), p.PlayerByName(s.player.Username()))
	}

	for i, done := range kicks {
		select {
		case <-done:
		case <-time.After(writeBudget + 8*time.Second):
			t.Fatalf("kick path of session %d never returned", i)
		}
	}
	require.Equal(t, players, p.PlayerCount())
}

// A playing session that is not being kicked must still never be displaced by a
// duplicate login; the release above must not weaken the duplicate check.
func TestPlayingSessionIsStillNotDisplacedByReconnect(t *testing.T) {
	p, hub, _ := newKickWedgeProxy(t, 200*time.Millisecond, "hub")
	defer hub.close()

	player, _ := placedPlayer(t, p, "KelsonGame", uuid.New(), 30*time.Second)
	duplicate := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "KelsonGame", player.ID())
	require.False(t, p.canRegisterConnection(duplicate))
	require.False(t, p.registerConnection(duplicate))
	require.Equal(t, Player(player), p.PlayerByName("KelsonGame"))
}

func waitForPlayerCountIn(t *testing.T, p *Proxy, want int, timeout time.Duration, logs *logCapture) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if p.PlayerCount() == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy player count = %d, want %d after %s: the registration of a kicked "+
				"player whose client stopped reading is leaked\nproxy log:\n%s",
				p.PlayerCount(), want, timeout, strings.Join(logs.snapshot(), "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

// blackHoleBackend accepts TCP connections and never answers the Minecraft
// handshake, the shape of a hub session that is accepted but never completes.
type blackHoleBackend struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func newBlackHoleBackend(t *testing.T) *blackHoleBackend {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	b := &blackHoleBackend{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			b.mu.Lock()
			b.conns = append(b.conns, conn)
			b.mu.Unlock()
		}
	}()
	return b
}

func (b *blackHoleBackend) addr() string { return b.ln.Addr().String() }

func (b *blackHoleBackend) close() {
	_ = b.ln.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.conns {
		_ = c.Close()
	}
}

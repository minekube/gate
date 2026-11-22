package proxy

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// These tests pin the duplicate-login behaviour that decides whether a player
// who reconnects after a backend kick, a client timeout or a network drop is
// refused with "You are already connected to this server!".
//
// Production evidence (Connect edge, 2026-10-17/18): the refusal fired while a
// previous registration for the same identity was either already closed
// (teardown racing the new login) or still logging in (never reached play).
// Both are abandoned attempts, not live player sessions, so they must not block
// the newer connection. A truly playing session must still be protected.

func newDuplicateLoginTestProxy() *Proxy {
	return &Proxy{
		log:         logr.Discard(),
		cfg:         &config.Config{},
		event:       event.New(),
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
		playerNames: map[string]*connectedPlayer{},
	}
}

type duplicateLoginTestConn struct {
	*testMinecraftConn
	ctx      context.Context
	cancel   context.CancelFunc
	registry *state.Registry
}

func newDuplicateLoginTestConn(registry *state.Registry) *duplicateLoginTestConn {
	ctx, cancel := context.WithCancel(context.Background())
	return &duplicateLoginTestConn{
		testMinecraftConn: &testMinecraftConn{},
		ctx:               ctx,
		cancel:            cancel,
		registry:          registry,
	}
}

func (c *duplicateLoginTestConn) Context() context.Context { return c.ctx }
func (c *duplicateLoginTestConn) Close() error             { c.cancel(); return nil }
func (c *duplicateLoginTestConn) State() *state.Registry   { return c.registry }

var _ netmc.MinecraftConn = (*duplicateLoginTestConn)(nil)

func newDuplicateLoginTestPlayer(
	p *Proxy,
	conn netmc.MinecraftConn,
	name string,
	id uuid.UUID,
) *connectedPlayer {
	return &connectedPlayer{
		MinecraftConn: conn,
		sessionHandlerDeps: &sessionHandlerDeps{
			proxy:          p,
			registrar:      p,
			eventMgr:       p.event,
			configProvider: p,
		},
		log: logr.Discard(),
		profile: &profile.GameProfile{
			ID:   id,
			Name: name,
		},
	}
}

// A connection that was never registered must not be able to remove the
// registration of the player that actually owns the name and UUID.
//
// This is a port of Velocity's `connectionsByName.remove(name, connection)`:
// the removal is value-scoped, so a refused duplicate's teardown cannot evict
// the live session it collided with.
func TestUnregisterConnectionKeepsOtherPlayersRegistration(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	live := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Play), "TheCoolKid71", id)
	require.True(t, p.registerConnection(live))

	// Same identity, but this connection lost the registration (refused duplicate).
	refused := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "TheCoolKid71", id)
	p.unregisterConnection(refused)

	require.Equal(t, 1, p.PlayerCount(), "refused duplicate must not unregister the live player")
	require.Equal(t, Player(live), p.PlayerByName("TheCoolKid71"))
	require.Equal(t, Player(live), p.Player(id))
}

// The reconnecting player supersedes an earlier attempt that never reached play
// (still logging in / connecting to its first server). This is the shape of the
// production failures: the earlier login was abandoned, its connection was not
// reaped yet, and the reconnect was refused.
func TestDuplicateLoginSupersedesUnestablishedSession(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	pending := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Config), "MinekubeK8s", id)
	require.True(t, p.canRegisterConnection(pending))
	require.True(t, p.registerConnection(pending))

	reconnectConn := newDuplicateLoginTestConn(state.Login)
	reconnect := newDuplicateLoginTestPlayer(p, reconnectConn, "MinekubeK8s", id)
	require.True(t, p.canRegisterConnection(reconnect),
		"a reconnect must not be refused while the previous attempt is still logging in")
	require.True(t, p.registerConnection(reconnect))

	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(reconnect), p.PlayerByName("MinekubeK8s"))
	require.Equal(t, Player(reconnect), p.Player(id))
	require.True(t, netmc.Closed(pending.MinecraftConn),
		"the superseded attempt must be closed, not left dangling")
}

// A registration whose connection is already closed but whose teardown has not
// unregistered it yet (the millisecond race on reconnect) must not block the
// reconnecting player either.
func TestDuplicateLoginSupersedesClosedRegistration(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	oldConn := newDuplicateLoginTestConn(state.Play)
	old := newDuplicateLoginTestPlayer(p, oldConn, "TheCoolKid71", id)
	require.True(t, p.registerConnection(old))
	require.NoError(t, oldConn.Close()) // connection gone, registry not yet cleaned

	reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "TheCoolKid71", id)
	require.True(t, p.canRegisterConnection(reconnect))
	require.True(t, p.registerConnection(reconnect))

	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(reconnect), p.PlayerByName("TheCoolKid71"))
}

// Bedrock players reach the Java proxy through the conversion layer as
// offline-mode profiles (UUID derived from the normalized username) and can
// reconnect right after a conversion/hand-off failure. The same supersede rule
// applies; the name lookup stays case-insensitive.
func TestBedrockReconnectSupersedesUnestablishedSession(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	name := "FiveKitten27327"
	id := uuid.OfflinePlayerUUID(name)

	pending := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Config), name, id)
	require.True(t, p.registerConnection(pending))

	reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), name, id)
	require.True(t, p.canRegisterConnection(reconnect))
	require.True(t, p.registerConnection(reconnect))

	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(reconnect), p.PlayerByName("fIVekitten27327"))
	require.True(t, netmc.Closed(pending.MinecraftConn))
}

// A second connection for an identity that is genuinely playing must still be
// refused: the duplicate check is the only protection against a live session
// being taken over by name.
func TestDuplicateLoginStillRefusedForEstablishedPlayingSession(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	playing := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Play), "Darianc23", id)
	require.True(t, p.registerConnection(playing))

	duplicate := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Darianc23", id)
	require.False(t, p.canRegisterConnection(duplicate))
	require.False(t, p.registerConnection(duplicate))

	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(playing), p.PlayerByName("Darianc23"))
	require.False(t, netmc.Closed(playing.MinecraftConn), "an established session must not be closed")
}

// A player who is already placed on a backend (e.g. mid server switch, where
// the client state is configuration again) still counts as established.
func TestDuplicateLoginRefusedWhileSwitchingServers(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	switching := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Config), "Darianc23", id)
	switching.connectedServer_ = &serverConnection{}
	require.True(t, p.registerConnection(switching))

	duplicate := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Darianc23", id)
	require.False(t, p.canRegisterConnection(duplicate))
	require.False(t, p.registerConnection(duplicate))
	require.Equal(t, Player(switching), p.PlayerByName("Darianc23"))
}

func TestDuplicateLoginSupersedeKeepsSessionIDStable(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	pending := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Config), "Nongroundbeef", id)
	require.True(t, p.registerConnection(pending))
	firstSessionID := p.sessionID()

	reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Nongroundbeef", id)
	require.True(t, p.registerConnection(reconnect))
	require.Equal(t, firstSessionID, p.sessionID(),
		"superseding a session must not reset the proxy session id")
}

// stuckWriteConn is a client connection that stopped reading: the farewell
// packet of a kick blocks in WritePacket until the test releases it, like a
// write to a half-open client with no write deadline installed. Like a real
// connection, Close runs the owning player's teardown (once), so the tests
// exercise the real unregister and DisconnectEvent order.
type stuckWriteConn struct {
	*duplicateLoginTestConn
	writes    atomic.Int32
	entered   chan struct{}
	release   chan struct{}
	teardown  func()
	closeOnce sync.Once
}

func newStuckWriteConn(registry *state.Registry) *stuckWriteConn {
	return &stuckWriteConn{
		duplicateLoginTestConn: newDuplicateLoginTestConn(registry),
		entered:                make(chan struct{}, 4),
		release:                make(chan struct{}),
	}
}

func (c *stuckWriteConn) WritePacket(proto.Packet) error {
	c.writes.Add(1)
	c.entered <- struct{}{}
	<-c.release
	return nil
}

func (c *stuckWriteConn) Close() error {
	_ = c.duplicateLoginTestConn.Close()
	c.closeOnce.Do(func() {
		if c.teardown != nil {
			c.teardown()
		}
	})
	return nil
}

// newStuckPlayer registers a playing session whose connection runs the player's
// real teardown on Close.
func newStuckPlayer(t *testing.T, p *Proxy, name string, id uuid.UUID) (*connectedPlayer, *stuckWriteConn) {
	t.Helper()
	conn := newStuckWriteConn(state.Play)
	player := newDuplicateLoginTestPlayer(p, conn, name, id)
	conn.teardown = player.teardown
	require.True(t, p.registerConnection(player))
	return player, conn
}

// startKick runs Disconnect on its own goroutine and waits until it is blocked
// in the farewell write.
func startKick(t *testing.T, player *connectedPlayer, conn *stuckWriteConn) (done <-chan struct{}) {
	t.Helper()
	kickDone := make(chan struct{})
	go func() {
		defer close(kickDone)
		player.Disconnect(internalServerConnectionError)
	}()
	select {
	case <-conn.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the kick never reached the farewell write")
	}
	return kickDone
}

func recordDisconnectStatuses(p *Proxy) <-chan LoginStatus {
	statuses := make(chan LoginStatus, 4)
	event.Subscribe(p.event, 0, func(e *DisconnectEvent) { statuses <- e.LoginStatus() })
	return statuses
}

// Production evidence (Connect edge, 2026-11-21): a player was kicked from the
// backend with "Internal server connection error" at 13:27:15 and from then on
// every login for that name was refused with "You are already connected to this
// server!" until the edge machine was replaced 21 hours later. The kick path
// (Disconnect -> CloseWith) closes the connection, and so runs the teardown
// that unregisters the player, only after the farewell write returns; that
// write never returned, and the registration still looked like a playing
// session.
//
// A session the proxy has already started to kick is on its way out. A
// reconnect must take over its registration immediately instead of waiting for
// the farewell write, and it must not queue a second disconnect behind the
// stuck write on the login goroutine.
func TestDuplicateLoginSupersedesSessionWhoseKickIsStuck(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	assertReconnectSupersedesStuckKick(t, p)
}

// With onlineModeKickExistingPlayers the duplicate is resolved by kicking the
// existing session instead. A session whose kick is already in flight must not
// be disconnected a second time there either: that call would queue behind the
// stuck farewell write on the new login's goroutine.
func TestDuplicateLoginKickExistingSupersedesSessionWhoseKickIsStuck(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	p.cfg = &config.Config{OnlineMode: true, OnlineModeKickExistingPlayers: true}
	assertReconnectSupersedesStuckKick(t, p)
}

func assertReconnectSupersedesStuckKick(t *testing.T, p *Proxy) {
	t.Helper()
	id := uuid.New()
	statuses := recordDisconnectStatuses(p)

	stuck, stuckConn := newStuckPlayer(t, p, "Tepurrrrr", id)
	kickDone := startKick(t, stuck, stuckConn)

	reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Tepurrrrr", id)
	require.True(t, p.canRegisterConnection(reconnect),
		"a reconnect must not be refused while the previous session's kick is in flight")

	registered := make(chan bool, 1)
	go func() { registered <- p.registerConnection(reconnect) }()
	select {
	case ok := <-registered:
		require.True(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("the reconnect blocked behind the stuck farewell write")
	}

	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(reconnect), p.PlayerByName("Tepurrrrr"))
	require.Equal(t, Player(reconnect), p.Player(id))
	require.EqualValues(t, 1, stuckConn.writes.Load(),
		"the superseded session must not be disconnected a second time")

	// The stuck write finally returns: the real close runs the old session's
	// teardown, which must not evict the connection that took over the
	// registration, and which reports the session as superseded.
	close(stuckConn.release)
	select {
	case <-kickDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the kick did not finish after its write returned")
	}
	select {
	case status := <-statuses:
		require.Equal(t, ConflictingLoginStatus, status,
			"a superseded session must report a conflict, like any other superseded session")
	case <-time.After(5 * time.Second):
		t.Fatal("the superseded session's teardown never fired DisconnectEvent")
	}
	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(reconnect), p.PlayerByName("Tepurrrrr"))
	require.Equal(t, Player(reconnect), p.Player(id))
}

// Only a session the proxy is actually kicking is superseded: a connection that
// is merely idle in play stays protected.
func TestDuplicateLoginStillRefusedForPlayingSessionThatIsNotBeingKicked(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	id := uuid.New()

	playing := newDuplicateLoginTestPlayer(p, newStuckWriteConn(state.Play), "Tepurrrrr", id)
	require.True(t, p.registerConnection(playing))
	require.False(t, playing.disconnecting.Load())

	duplicate := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Tepurrrrr", id)
	require.False(t, p.canRegisterConnection(duplicate))
	require.False(t, p.registerConnection(duplicate))
	require.Equal(t, Player(playing), p.PlayerByName("Tepurrrrr"))
}

// onlineModeKickExistingPlayers still kicks a session that is not being
// disconnected yet, exactly once, and the new login takes over.
func TestDuplicateLoginKickExistingStillKicksPlayingSession(t *testing.T) {
	p := newDuplicateLoginTestProxy()
	p.cfg = &config.Config{OnlineMode: true, OnlineModeKickExistingPlayers: true}
	id := uuid.New()
	statuses := recordDisconnectStatuses(p)

	playing, playingConn := newStuckPlayer(t, p, "Tepurrrrr", id)
	close(playingConn.release) // a healthy client: the farewell write returns

	reconnect := newDuplicateLoginTestPlayer(p, newDuplicateLoginTestConn(state.Login), "Tepurrrrr", id)
	require.True(t, p.registerConnection(reconnect))

	require.EqualValues(t, 1, playingConn.writes.Load())
	require.True(t, playing.disconnecting.Load())
	require.Equal(t, ConflictingLoginStatus, <-statuses)
	require.Equal(t, 1, p.PlayerCount())
	require.Equal(t, Player(reconnect), p.Player(id))
}

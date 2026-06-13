package proxy

import (
	"testing"
	"time"

	"github.com/dboslee/lru"
	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
)

// keepAliveTestConn is a MinecraftConn whose protocol state is configurable and
// which records written packets.
type keepAliveTestConn struct {
	*testMinecraftConn
	st      *state.Registry
	written []proto.Packet
}

func (c *keepAliveTestConn) State() *state.Registry { return c.st }
func (c *keepAliveTestConn) WritePacket(p proto.Packet) error {
	c.written = append(c.written, p)
	return nil
}

var _ netmc.MinecraftConn = (*keepAliveTestConn)(nil)

func newKeepAliveFixture(clientState, backendState *state.Registry) (*connectedPlayer, *serverConnection, *keepAliveTestConn) {
	client := &keepAliveTestConn{testMinecraftConn: &testMinecraftConn{}, st: clientState}
	player := &connectedPlayer{MinecraftConn: client, log: logr.Discard()}
	backend := &keepAliveTestConn{testMinecraftConn: &testMinecraftConn{}, st: backendState}
	sc := &serverConnection{
		player:       player,
		log:          logr.Discard(),
		pendingPings: lru.NewSync[int64, time.Time](lru.WithCapacity(5)),
	}
	sc.connection = backend
	return player, sc, backend
}

// When the client and backend are in the same (PLAY) state, the keep-alive is
// forwarded to the backend and consumed.
func TestSendKeepAliveForwardsWhenStatesMatch(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Play, state.Play)
	const id = int64(777)
	sc.pendingPings.Set(id, time.Now().Add(-5*time.Millisecond))

	if !sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: id}) {
		t.Fatal("expected keep-alive to be consumed (true)")
	}
	if len(backend.written) != 1 {
		t.Fatalf("expected keep-alive forwarded once, got %d writes", len(backend.written))
	}
}

// During 1.20.2+ server switches the client and backend can briefly be in
// different CONFIG/PLAY states. A matching pending ID is enough ownership proof:
// write the reply using the backend connection's current state.
func TestSendKeepAliveForwardsWhenClientPlayBackendConfig(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Play, state.Config)
	const id = int64(888)
	sc.pendingPings.Set(id, time.Now())

	if !sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: id}) {
		t.Fatal("expected keep-alive to be consumed (true)")
	}
	if len(backend.written) != 1 {
		t.Fatalf("expected keep-alive forwarded once, got %d writes", len(backend.written))
	}
}

func TestSendKeepAliveForwardsWhenClientConfigBackendPlay(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Config, state.Play)
	const id = int64(889)
	sc.pendingPings.Set(id, time.Now())

	if !sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: id}) {
		t.Fatal("expected keep-alive to be consumed (true)")
	}
	if len(backend.written) != 1 {
		t.Fatalf("expected keep-alive forwarded once, got %d writes", len(backend.written))
	}
}

// An unknown random id (no matching pending ping) is not ours: return false.
func TestSendKeepAliveIgnoresUnknownPing(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Play, state.Play)
	if sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: 999}) {
		t.Fatal("expected false for unknown ping id")
	}
	if len(backend.written) != 0 {
		t.Fatalf("expected no writes for unknown ping, got %d", len(backend.written))
	}
}

// Backends such as Minestom only accept a response to the latest keep-alive
// they sent. Keep older IDs out of the pending set so late Bedrock/Geyser
// replies are dropped instead of being forwarded to the backend and causing
// "Bad Keep Alive packet" kicks after rapid server switches.
func TestRecordBackendKeepAliveInvalidatesOlderPendingPings(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Play, state.Play)

	recordBackendKeepAlive(sc, &packet.KeepAlive{RandomID: 1})
	recordBackendKeepAlive(sc, &packet.KeepAlive{RandomID: 2})

	if sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: 1}) {
		t.Fatal("expected stale keep-alive response to be ignored")
	}
	if len(backend.written) != 0 {
		t.Fatalf("expected stale keep-alive not to be forwarded, got %d writes", len(backend.written))
	}

	if !sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: 2}) {
		t.Fatal("expected latest keep-alive response to be consumed")
	}
	if len(backend.written) != 1 {
		t.Fatalf("expected latest keep-alive forwarded once, got %d writes", len(backend.written))
	}
}

func TestForwardKeepAliveFallsBackToInFlightConnection(t *testing.T) {
	player, connected, connectedBackend := newKeepAliveFixture(state.Play, state.Play)
	_, inFlight, inFlightBackend := newKeepAliveFixture(state.Play, state.Config)
	inFlight.player = player
	player.connectedServer_ = connected
	player.connInFlight = inFlight

	const id = int64(42)
	inFlight.pendingPings.Set(id, time.Now())

	forwardKeepAlive(&packet.KeepAlive{RandomID: id}, player)

	if len(connectedBackend.written) != 0 {
		t.Fatalf("expected connected backend not to receive in-flight keep-alive, got %d writes", len(connectedBackend.written))
	}
	if len(inFlightBackend.written) != 1 {
		t.Fatalf("expected in-flight backend to receive keep-alive once, got %d writes", len(inFlightBackend.written))
	}
}

func TestBackendTransitionKeepAliveTracksAndForwardsToPlayer(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Play, state.Play)
	handler := &backendTransitionSessionHandler{serverConn: sc}

	handler.handleKeepAlive(&packet.KeepAlive{RandomID: 1234})

	if len(backend.written) != 0 {
		t.Fatalf("expected transition keep-alive not to be written back to backend, got %d writes", len(backend.written))
	}
	if len(player.MinecraftConn.(*keepAliveTestConn).written) != 1 {
		t.Fatalf("expected transition keep-alive forwarded to player once, got %d writes", len(player.MinecraftConn.(*keepAliveTestConn).written))
	}
	if _, ok := sc.pendingPings.Get(1234); !ok {
		t.Fatal("expected transition keep-alive to be tracked as pending")
	}
}

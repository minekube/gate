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

// When the backend is in a different state than the client (e.g. mid server
// switch the backend is in CONFIG while the client is in PLAY), the keep-alive
// must NOT be written to the backend (it would mis-encode), but it must still be
// consumed so it is not re-dispatched to another connection.
func TestSendKeepAliveDropsOnStateMismatch(t *testing.T) {
	player, sc, backend := newKeepAliveFixture(state.Play, state.Config)
	const id = int64(888)
	sc.pendingPings.Set(id, time.Now())

	if !sendKeepAliveToBackend(sc, player, &packet.KeepAlive{RandomID: id}) {
		t.Fatal("expected keep-alive to be consumed (true) even on mismatch")
	}
	if len(backend.written) != 0 {
		t.Fatalf("expected no forward on state mismatch, got %d writes", len(backend.written))
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

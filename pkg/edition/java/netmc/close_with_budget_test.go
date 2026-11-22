package netmc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// deadlineIgnoringConn is the client connection the Connect edge hands to Gate:
// a wrapper over a tunnel stream whose SetDeadline/SetReadDeadline/
// SetWriteDeadline report success without arming anything, because the stream
// underneath (a libp2p session stream) has no deadline support. moxy's
// session.streamConn does exactly that - it returns nil from the deadline
// setters when the wrapped stream implements no deadliner.
//
// Consequence on the wire: once the peer stops reading, a write blocks forever
// and the configured write budget never fires, because nothing was armed on the
// socket the write actually goes to.
type deadlineIgnoringConn struct{ net.Conn }

func (deadlineIgnoringConn) SetDeadline(time.Time) error      { return nil }
func (deadlineIgnoringConn) SetReadDeadline(time.Time) error  { return nil }
func (deadlineIgnoringConn) SetWriteDeadline(time.Time) error { return nil }

// A kick writes the farewell packet and only then closes the connection. The
// close is what runs the player teardown that unregisters the player, so a
// farewell write that never returns leaves a registration behind that refuses
// every later login for that name with "You are already connected to this
// server!" until the process is restarted.
//
// Production shape (Connect edge, 2026-11-22): five players were kicked from an
// endpoint at 13:59:44 after their session tunnel closed; three of them never
// logged "player has been disconnected" again and every reconnect was refused
// for the next 10+ minutes on a build that already had the real write budget
// installed, because the tunnel conn ignores SetWriteDeadline.
//
// The bound must therefore come from Gate, not from the socket: CloseWith has
// to close the connection itself once the connection's write budget expires,
// whether or not the write ever returns.
func TestCloseWithBoundsFarewellWriteWhenConnIgnoresDeadlines(t *testing.T) {
	const writeBudget = 200 * time.Millisecond

	proxySide, peerSide := net.Pipe() // the peer never reads: writes block
	defer peerSide.Close()

	conn := deadlineIgnoringConn{proxySide}
	mc, _ := NewMinecraftConn(context.Background(), conn, proto.ServerBound,
		0, writeBudget, -1, nil)
	mc.SetProtocol(version.Minecraft_1_21_9.Protocol)
	mc.Writer().SetState(state.Login)

	done := make(chan error, 1)
	go func() {
		done <- CloseWith(mc, packet.NewDisconnect(
			&component.Text{Content: "kicked"}, version.Minecraft_1_21_9.Protocol, state.Login.State))
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("CloseWith blocked on the farewell write for %s: the close, and with it the "+
			"teardown that unregisters the player, never runs", 5*time.Second)
	}

	require.True(t, Closed(mc), "the connection must be closed once the write budget is spent")
	kind, firedBudget, ok := DeadlineClose(mc)
	require.True(t, ok, "the close must be recorded as one of Gate's own deadlines")
	require.Equal(t, DeadlineWrite, kind)
	require.Equal(t, writeBudget, firedBudget)
	// The write had the whole budget to drain before the fallback close, so a
	// peer that is merely slow still receives the farewell.
}

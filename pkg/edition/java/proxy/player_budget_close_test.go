package proxy

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// playerDeadlineIgnoringConn reproduces the client connection the Connect edge
// hands Gate: moxy's tunnel streamConn whose deadline setters report success
// without arming anything, because the libp2p stream underneath has no deadline
// support. Once the peer stops reading, the write blocks forever.
type playerDeadlineIgnoringConn struct{ net.Conn }

func (playerDeadlineIgnoringConn) SetDeadline(time.Time) error      { return nil }
func (playerDeadlineIgnoringConn) SetReadDeadline(time.Time) error  { return nil }
func (playerDeadlineIgnoringConn) SetWriteDeadline(time.Time) error { return nil }

// A kick writes the farewell packet and only then closes the connection, and
// that close is what runs the teardown releasing the registration. On this edge
// the farewell write can never complete, so Gate has to close on its own write
// budget - and that close has to be *reportable through the player*, because
// the player is the connection the caller actually closed.
//
// The regression this pins: the fallback close type-asserted the concrete
// *minecraftConn while disconnectWithBudget passes *connectedPlayer, which
// embeds the MinecraftConn interface. The bound still fired (the fallback closes
// the wrapper, which forwards Close to the connection), but nothing was
// recorded: netmc.DeadlineClose on the player returned false, the netmc budget
// line was never written, and the "player has been disconnected" line reported
// no closeKind - i.e. a release Gate performed on its own budget was
// indistinguishable in production logs from the client simply going away.
func TestPlayerDisconnectOnWriteBudgetReportsItsCloseKind(t *testing.T) {
	const writeBudget = 150 * time.Millisecond

	proxySide, peerSide := net.Pipe() // the peer never reads: the farewell write blocks
	defer peerSide.Close()

	logs := &logCapture{}
	logger := funcr.New(logs.log, funcr.Options{Verbosity: 1})

	ctx := logr.NewContext(context.Background(), logger)
	mc, _ := netmc.NewMinecraftConn(ctx, playerDeadlineIgnoringConn{proxySide},
		proto.ServerBound, 0, writeBudget, -1, nil)
	mc.SetProtocol(version.Minecraft_1_21_9.Protocol)
	mc.Writer().SetState(state.Play)

	player := &connectedPlayer{
		MinecraftConn: mc,
		log:           logger,
		profile: &profile.GameProfile{
			ID:   uuid.New(),
			Name: "Alson", // the name the Connect edge refused on every reconnect
		},
	}

	start := time.Now()
	player.Disconnect(&component.Text{Content: "kicked"})
	elapsed := time.Since(start)

	require.Less(t, elapsed, 2*time.Second,
		"Disconnect must return once the write budget is spent instead of holding the registration")
	require.True(t, netmc.Closed(mc), "the connection must be closed once the write budget is spent")
	captured := logs.snapshot()
	t.Logf("captured logs: %q", captured)

	// Read the cause off the connection the caller closed: the player.
	kind, firedBudget, ok := netmc.DeadlineClose(player)
	require.True(t, ok, "a close Gate performed on its write budget must stay reportable through the player connection")
	require.Equal(t, netmc.DeadlineWrite, kind)
	require.Equal(t, writeBudget, firedBudget)

	// One episode, one budget line, and the player's disconnect names the cause
	// so an operator can tell our release apart from a client that went away.
	budgetLines := 0
	disconnectLine := ""
	for _, line := range captured {
		if strings.Contains(line, "farewell write exceeded the write budget") {
			budgetLines++
		}
		if strings.Contains(line, "player has been disconnected") {
			disconnectLine = line
		}
	}
	require.Equal(t, 1, budgetLines, "the budget close is logged once per episode")
	require.Contains(t, disconnectLine, "closeKind")
	require.Contains(t, disconnectLine, string(netmc.DeadlineWrite))
	require.Contains(t, disconnectLine, "budget")
}

package proxy

import (
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// recordingConn is a MinecraftConn with a configurable protocol version that
// records the packets written to it in a thread-safe way (the chat queue writes
// from a background goroutine).
type recordingConn struct {
	*testMinecraftConn
	protocol proto.Protocol
	mu       sync.Mutex
	packets  []proto.Packet
}

func (c *recordingConn) Protocol() proto.Protocol { return c.protocol }

func (c *recordingConn) WritePacket(p proto.Packet) error {
	c.mu.Lock()
	c.packets = append(c.packets, p)
	c.mu.Unlock()
	return nil
}

func (c *recordingConn) written() []proto.Packet {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]proto.Packet(nil), c.packets...)
}

var _ netmc.MinecraftConn = (*recordingConn)(nil)

type fakeConfigProvider struct{ cfg *config.Config }

func (f fakeConfigProvider) config() *config.Config { return f.cfg }

// newChatCmdFixture wires a connectedPlayer to a fake backend so the real
// command/chat handler and chat queue can be driven end-to-end. configureEvent
// controls the CommandExecuteEvent outcome (e.g. forward or deny) so we can
// exercise the packet actually sent to the backend.
func newChatCmdFixture(protocol proto.Protocol, configureEvent func(*CommandExecuteEvent)) (*connectedPlayer, *recordingConn, *chatHandler) {
	client := &recordingConn{testMinecraftConn: &testMinecraftConn{}, protocol: protocol}
	player := &connectedPlayer{MinecraftConn: client, log: logr.Discard()}
	player.chatQueue = newChatQueue(player)

	backend := &recordingConn{testMinecraftConn: &testMinecraftConn{}, protocol: protocol}
	sc := &serverConnection{player: player, log: logr.Discard()}
	sc.connection = backend
	player.connectedServer_ = sc

	eventMgr := event.New()
	event.Subscribe(eventMgr, 0, configureEvent)

	h := &chatHandler{
		log:            logr.Discard(),
		eventMgr:       eventMgr,
		player:         player,
		configProvider: fakeConfigProvider{cfg: &config.Config{}},
	}
	return player, backend, h
}

func forwardEvent(e *CommandExecuteEvent) { e.SetForward(true) }
func denyEvent(e *CommandExecuteEvent)    { e.SetAllowed(false) }

// TestUnsignedCommandPreservesHeldAcknowledgements reproduces the 1.21.x
// "message acknowledgement" kick (gate#915 / gate#921).
//
// On 1.20.5+, a command with no signable arguments is sent as an
// UnsignedPlayerCommand, which carries NO last-seen update - exactly like a
// direct connection and like Velocity's UnsignedPlayerCommandPacket (whose
// lastSeenMessages is null). Gate instead fed a non-nil zero-value last-seen
// into the chat queue, which:
//
//  1. flushed the player's held ChatAcknowledgements (delayedAckCount) and then
//     discarded them, so the backend's last-seen window fell behind the client's
//     and the next chat/command was rejected with
//     "Last seen update ignored previously acknowledged message at index N"; and
//  2. re-encoded the command as a signed session command carrying an empty
//     acknowledgement, which the backend validates (and rejects) even though a
//     direct connection sends no acknowledgement at all.
//
// This test fails against the pre-fix code.
func TestUnsignedCommandPreservesHeldAcknowledgements(t *testing.T) {
	player, backend, chatHandler := newChatCmdFixture(version.Minecraft_1_21_5.Protocol, forwardEvent)
	h := &clientPlaySessionHandler{
		log:         logr.Discard(),
		player:      player,
		chatHandler: chatHandler,
	}
	cq := h.player.chatQueue

	// The client acknowledged 3 messages via ChatAcknowledgement packets that the
	// proxy holds back (well below the 20-message window). These must not be lost.
	cq.HandleAcknowledgement(3)

	// A 1.20.5+ client runs a plain command (no signable args): an
	// UnsignedPlayerCommand carrying no last-seen update.
	h.HandlePacket(&proto.PacketContext{
		Protocol: version.Minecraft_1_21_5.Protocol,
		Packet: &chat.UnsignedPlayerCommand{
			SessionPlayerCommand: chat.SessionPlayerCommand{Command: "spawn"},
		},
	})

	// Wait for the command to reach the backend.
	require.Eventually(t, func() bool { return len(backend.written()) == 1 },
		2*time.Second, 5*time.Millisecond, "command was never forwarded to backend")

	// (1) The held acknowledgements must be preserved, not flushed-and-discarded.
	require.Equal(t, int32(3), cq.chatState.delayedAckCount.Load(),
		"held acknowledgements were lost when forwarding an unsigned command")

	// (2) The command must be forwarded command-only (UnsignedPlayerCommand), not
	// re-encoded as a signed session command with a spurious empty last-seen update.
	got := backend.written()[0]
	_, ok := got.(*chat.UnsignedPlayerCommand)
	require.Truef(t, ok, "expected *chat.UnsignedPlayerCommand forwarded, got %T", got)
}

// TestConsumedCommandAcknowledgesOffsetWithEmptyBitset covers the second
// divergence from Velocity: when the proxy consumes a command (here a plugin
// denies it) on a pre-1.20.5 signed-session client, it must forward a
// ChatAcknowledgement carrying the command's last-seen offset so the backend
// advances its window. Gate previously gated this on the acknowledged bitset
// being non-empty; a ChatAcknowledgement only carries an offset, so a command
// with a non-zero offset but an empty bitset was dropped, desyncing the backend.
// This matches Velocity's SessionCommandHandler.consumeCommand (offset != 0).
//
// This test fails against the pre-fix code.
func TestConsumedCommandAcknowledgesOffsetWithEmptyBitset(t *testing.T) {
	// 1.20.3: commands are SessionPlayerCommand carrying a last-seen update (no
	// UnsignedPlayerCommand until 1.20.5), with no argument signatures here.
	_, backend, h := newChatCmdFixture(version.Minecraft_1_20_3.Protocol, denyEvent)

	// A consumed command whose last-seen update advances the window (offset 4) but
	// acknowledges no in-window messages (empty bitset).
	require.NoError(t, h.handleCommand(&chat.SessionPlayerCommand{
		Command:          "hub",
		LastSeenMessages: chat.LastSeenMessages{Offset: 4},
	}))

	require.Eventually(t, func() bool { return len(backend.written()) == 1 },
		2*time.Second, 5*time.Millisecond, "no acknowledgement was forwarded for the consumed command")

	got := backend.written()[0]
	ack, ok := got.(*chat.ChatAcknowledgement)
	require.Truef(t, ok, "expected *chat.ChatAcknowledgement forwarded, got %T", got)
	require.Equal(t, 4, ack.Offset, "acknowledgement must carry the command's last-seen offset")
}

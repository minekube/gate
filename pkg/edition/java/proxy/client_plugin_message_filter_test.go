package proxy

import (
	"net"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
)

type clientPluginMessageFilterFunc func(channel string, player Player, target RegisteredServer) bool

func (f clientPluginMessageFilterFunc) AllowClientPluginMessage(
	channel string, player Player, target RegisteredServer,
) bool {
	return f(channel, player, target)
}

// newClientPluginMessageFixture wires a player in play state to a recording
// backend so that clientPlaySessionHandler.handlePluginMessage runs its real
// forwarding branch: both connection phases are complete and the backend
// connection is in the play state.
func newClientPluginMessageFixture(t *testing.T) (*Proxy, *clientPlaySessionHandler, *recordingConn) {
	t.Helper()

	protocol := version.Minecraft_1_21_5.Protocol
	p := &Proxy{log: logr.Discard(), channelRegistrar: message.NewChannelRegistrar()}

	client := &recordingConn{testMinecraftConn: &testMinecraftConn{}, protocol: protocol}
	backend := &recordingConn{testMinecraftConn: &testMinecraftConn{}, protocol: protocol}

	player := &connectedPlayer{
		MinecraftConn:      client,
		sessionHandlerDeps: &sessionHandlerDeps{proxy: p},
		log:                logr.Discard(),
		connPhase:          phase.VanillaClientPhase,
	}

	sc := &serverConnection{
		server:    newRegisteredServer(NewServerInfo("paper", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25566})),
		player:    player,
		log:       logr.Discard(),
		connPhase: phase.VanillaBackendPhase,
	}
	sc.connection = backend
	player.connectedServer_ = sc

	h := &clientPlaySessionHandler{log: logr.Discard(), player: player}
	return p, h, backend
}

func forwardedPluginMessages(c *recordingConn) []string {
	var channels []string
	for _, p := range c.written() {
		if msg, ok := p.(*plugin.Message); ok {
			channels = append(channels, msg.Channel)
		}
	}
	return channels
}

// A filter that denies a channel must keep the message away from the backend
// without disconnecting the player. This is the regression for the backend
// Floodgate kick: Geyser sends floodgate:skin one second after spawn, the
// backend Floodgate cannot resolve Gate's XUID-derived v5 identity, and its
// only answer to an unknown player on its channels is to kick the session.
func TestClientPluginMessageDroppedWhenFilterDenies(t *testing.T) {
	p, h, backend := newClientPluginMessageFixture(t)

	var seen []string
	p.SetClientPluginMessageFilter(clientPluginMessageFilterFunc(
		func(channel string, _ Player, _ RegisteredServer) bool {
			seen = append(seen, channel)
			return false
		},
	))

	h.handlePluginMessage(&plugin.Message{Channel: "floodgate:skin", Data: []byte("skin")})

	require.Empty(t, forwardedPluginMessages(backend),
		"a denied client plugin message must never reach the backend")
	require.Equal(t, []string{"floodgate:skin"}, seen, "filter must see the channel the client sent")
}

// The filter is consulted for every client plugin message, and allowing one must
// leave forwarding exactly as it was: the payload still reaches the backend on
// the same channel.
func TestClientPluginMessageForwardedWhenFilterAllows(t *testing.T) {
	p, h, backend := newClientPluginMessageFixture(t)

	var seen []string
	p.SetClientPluginMessageFilter(clientPluginMessageFilterFunc(
		func(channel string, _ Player, _ RegisteredServer) bool {
			seen = append(seen, channel)
			return true
		},
	))

	h.handlePluginMessage(&plugin.Message{Channel: "floodgate:skin", Data: []byte("skin")})

	require.Equal(t, []string{"floodgate:skin"}, seen)
	require.Equal(t, []string{"floodgate:skin"}, forwardedPluginMessages(backend),
		"an allowed client plugin message must be forwarded unchanged")
}

// Without a registered filter nothing changes: the message is forwarded.
func TestClientPluginMessageForwardedWithoutFilter(t *testing.T) {
	_, h, backend := newClientPluginMessageFixture(t)

	h.handlePluginMessage(&plugin.Message{Channel: "floodgate:skin", Data: []byte("skin")})

	require.Equal(t, []string{"floodgate:skin"}, forwardedPluginMessages(backend))
}

// Unregistering restores the default, so a stopped integration cannot leave a
// stale deny rule behind for the next configuration.
func TestClientPluginMessageFilterUnregister(t *testing.T) {
	p, h, backend := newClientPluginMessageFixture(t)

	unregister := p.SetClientPluginMessageFilter(clientPluginMessageFilterFunc(
		func(string, Player, RegisteredServer) bool { return false },
	))
	h.handlePluginMessage(&plugin.Message{Channel: "floodgate:skin"})
	require.Empty(t, forwardedPluginMessages(backend))

	unregister()
	h.handlePluginMessage(&plugin.Message{Channel: "floodgate:skin"})
	require.Equal(t, []string{"floodgate:skin"}, forwardedPluginMessages(backend))
}

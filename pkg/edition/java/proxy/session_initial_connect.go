package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/gate/proto"
)

type initialConnectSessionHandler struct {
	player *connectedPlayer

	nopSessionHandler
}

func newInitialConnectSessionHandler(player *connectedPlayer) sessionHandler {
	return &initialConnectSessionHandler{player: player}
}

func (i *initialConnectSessionHandler) handlePacket(p *proto.PacketContext) {
	if !p.KnownPacket {
		return
	}
	switch typed := p.Packet.(type) {
	case *plugin.Message:
		i.handlePluginMessage(typed)
	}
}

func (i *initialConnectSessionHandler) handlePluginMessage(packet *plugin.Message) {
	serverConn := i.player.connectionInFlight()
	if serverConn == nil {
		return // Do nothing
	}
	if i.player.phase().handle(serverConn, packet) {
		return // Done
	}

	if plugin.Register(packet) {
		i.player.pluginChannelsMu.Lock()
		i.player.pluginChannels.Insert(plugin.Channels(packet)...)
		i.player.pluginChannelsMu.Unlock()
	} else if plugin.Unregister(packet) {
		i.player.pluginChannelsMu.Lock()
		i.player.pluginChannels.Delete(plugin.Channels(packet)...)
		i.player.pluginChannelsMu.Unlock()
	}
	if serverConn.player.Active() {
		_ = serverConn.player.WritePacket(packet)
	}
}

func (i *initialConnectSessionHandler) disconnected() {
	// Just after we registered the player connection,
	// the user canceled login process or
	// we did not find an initial server to connect the player to
	// or due to something else.
	i.player.teardown()
}

var _ sessionHandler = (*initialConnectSessionHandler)(nil)

func (i *initialConnectSessionHandler) player_() *connectedPlayer {
	return i.player
}

package proxy

import (
	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/gate/proto"
)

type initialConnectSessionHandler struct {
	player *connectedPlayer

	nopSessionHandler
}

func newInitialConnectSessionHandler(player *connectedPlayer) netmc.SessionHandler {
	return &initialConnectSessionHandler{player: player}
}

func (i *initialConnectSessionHandler) HandlePacket(p *proto.PacketContext) {
	if !p.KnownPacket() {
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
	if phaseHandle(i.player, serverConn.conn(), packet) {
		return // Done
	}

	if plugin.IsRegister(packet) {
		i.player.pluginChannelsMu.Lock()
		i.player.pluginChannels.Insert(plugin.Channels(packet)...)
		i.player.pluginChannelsMu.Unlock()
	} else if plugin.IsUnregister(packet) {
		i.player.pluginChannelsMu.Lock()
		i.player.pluginChannels.Delete(plugin.Channels(packet)...)
		i.player.pluginChannelsMu.Unlock()
	}
	if serverConn.player.Active() {
		_ = serverConn.player.WritePacket(packet)
	}
}

func (i *initialConnectSessionHandler) Disconnected() {
	// Just after we registered the player connection,
	// the user canceled login process or
	// we did not find an initial server to connect the player to
	// or due to something else.
	i.player.teardown()
}

var _ netmc.SessionHandler = (*initialConnectSessionHandler)(nil)

func (i *initialConnectSessionHandler) PlayerLog() logr.Logger {
	return i.player.log
}

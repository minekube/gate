package proxy

import (
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
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

	if bungeecord.IsBungeeCordMessage(packet) {
		return
	}

	id, ok := i.player.proxy.ChannelRegistrar().FromID(packet.Channel)
	if !ok {
		if serverMc, ok := serverConn.ensureConnected(); ok {
			_ = serverMc.WritePacket(packet)
		}
		return
	}

	clone := make([]byte, len(packet.Data))
	copy(clone, packet.Data)
	event.FireParallel(i.player.eventMgr, &PluginMessageEvent{
		source:     serverConn,
		target:     serverConn.player,
		identifier: id,
		data:       clone,
		forward:    true,
	}, func(pme *PluginMessageEvent) {
		if pme.Allowed() && serverConn.active() {
			serverMc, ok := serverConn.ensureConnected()
			if ok {
				_ = serverMc.WritePacket(&plugin.Message{
					Channel: packet.Channel,
					Data:    pme.Data(),
				})
			}
		}
	})
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

package proxy

import (
	"errors"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/util/sets"
	"go.uber.org/atomic"
)

type backendPlaySessionHandler struct {
	serverConn                *serverConnection
	bungeeCordMessageRecorder *bungeeCordMessageRecorder
	exceptionTriggered        atomic.Bool

	nopSessionHandler
}

func newBackendPlaySessionHandler(serverConn *serverConnection) (sessionHandler, error) {
	_, ok := serverConn.player.SessionHandler().(*clientPlaySessionHandler)
	if !ok {
		return nil, errors.New("initializing backendPlaySessionHandler with no backing client play session handler")
	}
	return &backendPlaySessionHandler{
		serverConn:                serverConn,
		bungeeCordMessageRecorder: &bungeeCordMessageRecorder{ConnectedPlayer: serverConn.player},
	}, nil
}

func (b *backendPlaySessionHandler) handlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket {
		// forward unknown packet to player
		b.forwardToPlayer(pc, nil)
		return
	}
	if !b.shouldHandle() {
		return
	}
	switch p := pc.Packet.(type) {
	case *packet.KeepAlive:
		b.handleKeepAlive(p, pc)
	case *packet.Disconnect:
		b.handleDisconnect(p)
	case *plugin.Message:
		b.handlePluginMessage(p, pc)
	case *packet.PlayerListItem:
		b.handlePlayerListItem(p, pc)
	default:
		b.forwardToPlayer(pc, nil)
	}
}

func (b *backendPlaySessionHandler) shouldHandle() bool {
	if b.serverConn.active() {
		return true
	}
	// Obsolete connection
	b.serverConn.disconnect()
	return false
}

func (b *backendPlaySessionHandler) activated() {
	b.serverConn.server.players.add(b.serverConn.player)
	serverMc, ok := b.serverConn.ensureConnected()
	if ok {
		protocol := serverMc.Protocol()
		channelsPacket := plugin.ConstructChannelsPacket(protocol, b.bungeeCordMessageRecorder.bungeeCordChannel(protocol))
		_ = serverMc.WritePacket(channelsPacket)
	}
}

func (b *backendPlaySessionHandler) disconnected() {
	b.serverConn.server.players.remove(b.serverConn.player)
	if b.serverConn.gracefulDisconnect.Load() || b.exceptionTriggered.Load() {
		return
	}
	if b.proxy().Config().FailoverOnUnexpectedServerDisconnect {
		b.serverConn.player.handleDisconnectWithReason(b.serverConn.server, internalServerConnectionError, true)
	} else {
		b.serverConn.player.Disconnect(internalServerConnectionError)
	}
}

func (b *backendPlaySessionHandler) handleKeepAlive(p *packet.KeepAlive, pc *proto.PacketContext) {
	b.serverConn.lastPingID.Store(p.RandomID)
	b.forwardToPlayer(pc, nil) // forward on
}

func (b *backendPlaySessionHandler) handleDisconnect(p *packet.Disconnect) {
	b.serverConn.disconnect()
	b.serverConn.player.handleDisconnect(b.serverConn.server, p, true)
}

func (b *backendPlaySessionHandler) handlePluginMessage(packet *plugin.Message, pc *proto.PacketContext) {
	if b.bungeeCordMessageRecorder.process(packet) {
		return
	}

	serverMc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}

	serverVersion := serverMc.Protocol()
	if !b.serverConn.player.canForwardPluginMessage(serverVersion, packet) {
		return
	}

	// We need to specially handle REGISTER and UNREGISTER packets.
	// Later on, we'll write them to the client.
	if plugin.Register(packet) {
		b.serverConn.player.lockedKnownChannels(func(knownChannels sets.String) {
			knownChannels.Insert(plugin.Channels(packet)...)
		})
		b.forwardToPlayer(pc, nil)
		return
	} else if plugin.Unregister(packet) {
		b.serverConn.player.lockedKnownChannels(func(knownChannels sets.String) {
			knownChannels.Delete(plugin.Channels(packet)...)
		})
		b.forwardToPlayer(pc, nil)
		return
	}

	if plugin.McBrand(packet) {
		rewritten := plugin.RewriteMinecraftBrand(packet, serverVersion)
		b.forwardToPlayer(nil, rewritten)
		return
	}

	if b.serverConn.phase().handle(b.serverConn, packet) {
		return // handled
	}

	id, ok := b.proxy().ChannelRegistrar().FromID(packet.Channel)
	if !ok {
		b.forwardToPlayer(pc, nil)
		return
	}

	clone := make([]byte, len(packet.Data))
	copy(clone, packet.Data)
	b.proxy().Event().FireParallel(&PluginMessageEvent{
		source:     b.serverConn,
		target:     b.serverConn.player,
		identifier: id,
		data:       clone,
		forward:    true,
	}, func(e event.Event) {
		pme := e.(*PluginMessageEvent)
		if pme.Allowed() && b.serverConn.player.Active() {
			b.forwardToPlayer(nil, &plugin.Message{
				Channel: packet.Channel,
				Data:    clone,
			})
		}
	})
}

func (b *backendPlaySessionHandler) handlePlayerListItem(p *packet.PlayerListItem, pc *proto.PacketContext) {
	b.serverConn.player.tabList.processBackendPacket(p)
	b.forwardToPlayer(pc, nil)
}

// prefer PacketContext over Packet
//
// since we already have the packet's payload we can simply forward it on,
// instead of encoding a Packet again each time.
//
// This increases throughput & decreases CPU and memory usage
func (b *backendPlaySessionHandler) forwardToPlayer(packetContext *proto.PacketContext, packet proto.Packet) {
	if packetContext == nil {
		_ = b.serverConn.player.WritePacket(packet)
		return
	}
	_ = b.serverConn.player.Write(packetContext.Payload)
}

func (b *backendPlaySessionHandler) proxy() *Proxy {
	return b.serverConn.player.proxy
}

var _ sessionHandler = (*backendPlaySessionHandler)(nil)

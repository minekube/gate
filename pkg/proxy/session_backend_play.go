package proxy

import (
	"errors"
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/proto/packet/plugin"
	"go.minekube.com/gate/pkg/proxy/message"
	"go.minekube.com/gate/pkg/util/sets"
	"go.uber.org/atomic"
	"strings"
)

type backendPlaySessionHandler struct {
	serverConn                *serverConnection
	playerSessionHandler      *clientPlaySessionHandler
	bungeeCordMessageRecorder *bungeeCordMessageRecorder
	exceptionTriggered        atomic.Bool

	noOpSessionHandler
}

func newBackendPlaySessionHandler(serverConn *serverConnection) (sessionHandler, error) {
	psh, ok := serverConn.player.SessionHandler().(*clientPlaySessionHandler)
	if !ok {
		return nil, errors.New("initializing backendPlaySessionHandler with no backing client play session handler")
	}
	return &backendPlaySessionHandler{
		serverConn:                serverConn,
		playerSessionHandler:      psh,
		bungeeCordMessageRecorder: &bungeeCordMessageRecorder{player: serverConn.player},
	}, nil
}

func (b *backendPlaySessionHandler) handlePacket(pack proto.Packet) {
	if !b.shouldHandle() {
		return
	}
	switch p := pack.(type) {
	case *packet.KeepAlive:
		b.handleKeepAlive(p)
	case *packet.Disconnect:
		b.handleDisconnect(p)
	case *plugin.Message:
		b.handlePluginMessage(p)
	default:
		b.forwardToPlayer(pack)
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
		b.serverConn.player.handleDisconnect(b.serverConn.server,
			packet.DisconnectWithProtocol(internalServerConnectionError, b.serverConn.player.Protocol()),
			true)
	} else {
		b.serverConn.player.Disconnect(internalServerConnectionError)
	}
}

func (b *backendPlaySessionHandler) handleKeepAlive(p *packet.KeepAlive) {
	b.serverConn.lastPingId.Store(p.RandomId)
	b.forwardToPlayer(p) // forwards on
}

func (b *backendPlaySessionHandler) handleDisconnect(p *packet.Disconnect) {
	b.serverConn.disconnect()
	b.serverConn.player.handleDisconnect(b.serverConn.server, p, true)
}

func (b *backendPlaySessionHandler) handlePluginMessage(packet *plugin.Message) {
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
		b.forwardToPlayer(packet)
		return
	} else if plugin.Unregister(packet) {
		b.serverConn.player.lockedKnownChannels(func(knownChannels sets.String) {
			knownChannels.Delete(plugin.Channels(packet)...)
		})
		b.forwardToPlayer(packet)
		return
	}

	if plugin.McBrand(packet) {
		rewritten := plugin.RewriteMinecraftBrand(packet, serverVersion)
		b.forwardToPlayer(rewritten)
		return
	}

	if b.serverConn.phase().handle(b.serverConn, packet) {
		return // handled
	}

	id, ok := b.proxy().ChannelRegistrar().FromId(packet.Channel)
	if !ok {
		b.forwardToPlayer(packet)
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
			b.forwardToPlayer(&plugin.Message{
				Channel: packet.Channel,
				Data:    clone,
			})
		}
	})
}

func (b *backendPlaySessionHandler) forwardToPlayer(p proto.Packet) {
	_ = b.serverConn.player.WritePacket(p)
}

func (b *backendPlaySessionHandler) handleUnknownPacket(p *proto.PacketContext) {
	_ = b.serverConn.player.Write(p.Payload) // forward to player
}

func (b *backendPlaySessionHandler) proxy() *Proxy {
	return b.serverConn.player.proxy
}

var _ sessionHandler = (*backendPlaySessionHandler)(nil)

type bungeeCordMessageRecorder struct {
	player *connectedPlayer
}

var (
	bungeeCordModernChannel = &message.MinecraftChannelIdentifier{Key: key.New("bungeecord", "main")}
	bungeeCordLegacyChannel = message.LegacyChannelIdentifier("Bungeecord")
)

func (r *bungeeCordMessageRecorder) bungeeCordChannel(protocol proto.Protocol) string {
	if protocol.GreaterEqual(proto.Minecraft_1_13) {
		return bungeeCordModernChannel.Id()
	}
	return bungeeCordLegacyChannel.Id()
}

func (r *bungeeCordMessageRecorder) process(message *plugin.Message) bool {
	if !r.config().BungeePluginChannelEnabled {
		return false
	}

	if !strings.EqualFold(bungeeCordModernChannel.Id(), message.Channel) &&
		!strings.EqualFold(bungeeCordLegacyChannel.Id(), message.Channel) {
		return false
	}

	return true
	// TODO support bungeecord plugin channels
	//in := UTFReader(message.Data)
	//subChannel := in.readUTF() // read first sequence
	//switch subChannel {
	//case "ForwardToPlayer":
	//	r.forwardToPlayer()
	// ...more: https://www.spigotmc.org/wiki/bukkit-bungee-plugin-messaging-channel/
	//}
}

func (r *bungeeCordMessageRecorder) config() *config.Config {
	return r.player.proxy.config
}

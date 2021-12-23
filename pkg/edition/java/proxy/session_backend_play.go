package proxy

import (
	"context"
	"errors"
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/command"
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
	playerSessionHandler      *clientPlaySessionHandler

	nopSessionHandler
}

func newBackendPlaySessionHandler(serverConn *serverConnection) (sessionHandler, error) {
	cpsh, ok := serverConn.player.SessionHandler().(*clientPlaySessionHandler)
	if !ok {
		return nil, errors.New("initializing backendPlaySessionHandler with no backing client play session handler")
	}
	return &backendPlaySessionHandler{
		serverConn:                serverConn,
		bungeeCordMessageRecorder: &bungeeCordMessageRecorder{connectedPlayer: serverConn.player},
		playerSessionHandler:      cpsh,
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
	case *packet.AvailableCommands:
		b.handleAvailableCommands(p)
	case *packet.TabCompleteResponse:
		b.playerSessionHandler.handleTabCompleteResponse(p)
	case *packet.PlayerListItem:
		b.handlePlayerListItem(p, pc)
	// TODO case *packet.ResourcePackRequest:
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
	if b.proxy().config.BungeePluginChannelEnabled {
		serverMc, ok := b.serverConn.ensureConnected()
		if ok {
			protocol := serverMc.Protocol()
			channelsPacket := plugin.ConstructChannelsPacket(protocol,
				b.bungeeCordMessageRecorder.bungeeCordChannel(protocol))
			_ = serverMc.WritePacket(channelsPacket)
		}
	}
}

func (b *backendPlaySessionHandler) disconnected() {
	b.serverConn.server.players.remove(b.serverConn.player)
	if b.serverConn.gracefulDisconnect.Load() || b.exceptionTriggered.Load() {
		return
	}
	if b.proxy().Config().FailoverOnUnexpectedServerDisconnect {
		b.serverConn.player.handleDisconnectWithReason(b.serverConn.server,
			internalServerConnectionError, true)
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

	// We need to specially handle REGISTER and UNREGISTER packets.
	// Later on, we'll write them to the client.
	if plugin.IsRegister(packet) {
		b.serverConn.player.lockedKnownChannels(func(knownChannels sets.String) {
			knownChannels.Insert(plugin.Channels(packet)...)
		})
		b.forwardToPlayer(pc, nil)
		return
	} else if plugin.IsUnregister(packet) {
		b.serverConn.player.lockedKnownChannels(func(knownChannels sets.String) {
			knownChannels.Delete(plugin.Channels(packet)...)
		})
		b.forwardToPlayer(pc, nil)
		return
	}

	if plugin.McBrand(packet) {
		serverMc, ok := b.serverConn.ensureConnected()
		if !ok {
			return
		}
		serverVersion := serverMc.Protocol()
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
	// Track changes to tab list of player
	if err := b.serverConn.player.tabList.processBackendPacket(p); err != nil {
		b.serverConn.log.Error(err, "Error while processing backend PlayerListItem packet, ignored")
	}
	b.forwardToPlayer(pc, nil)
}

func (b *backendPlaySessionHandler) handleAvailableCommands(p *packet.AvailableCommands) {
	rootNode := p.RootNode
	if b.proxy().config.AnnounceProxyCommands {
		// Inject commands from the proxy.
		dispatcherRootNode := filterNode(&b.proxy().command.Root, b.serverConn.player)
		if dispatcherRootNode == nil {
			return // unexpected
		}
		proxyNodes := dispatcherRootNode.ChildrenOrdered()
		proxyNodes.Range(func(_ string, node brigodier.CommandNode) bool {
			existingServerChild := rootNode.Children()[node.Name()]
			if existingServerChild != nil {
				rootNode.RemoveChild(existingServerChild.Name())
			}
			rootNode.AddChild(node)
			return true
		})
	}

	b.proxy().event.Fire(&PlayerAvailableCommandsEvent{
		player:   b.serverConn.player,
		rootNode: rootNode,
	})
	_ = b.serverConn.player.WritePacket(p)
}

func filterNode(src brigodier.CommandNode, cmdSrc command.Source) brigodier.CommandNode {
	var dest brigodier.CommandNode
	_, ok := src.(*brigodier.RootCommandNode)
	if ok {
		dest = &brigodier.RootCommandNode{}
	} else {
		if !src.CanUse(command.ContextWithSource(context.Background(), cmdSrc)) {
			return nil
		}
		builder := src.CreateBuilder().Requires(func(context.Context) bool { return true })
		if src.Redirect() != nil {
			builder.Redirect(filterNode(src.Redirect(), cmdSrc))
		}
		dest = builder.Build()
	}

	src.ChildrenOrdered().Range(func(_ string, sourceChild brigodier.CommandNode) bool {
		destChild := filterNode(sourceChild, cmdSrc)
		if destChild != nil {
			dest.AddChild(destChild)
		}
		return true
	})

	return dest
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

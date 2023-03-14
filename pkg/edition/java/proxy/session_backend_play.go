package proxy

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"regexp"
	"time"

	"github.com/robinbraemer/event"
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/bossbar"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/sets"
	"go.uber.org/atomic"
)

type backendPlaySessionHandler struct {
	serverConn                 *serverConnection
	bungeeCordMessageResponder bungeecord.MessageResponder
	exceptionTriggered         atomic.Bool
	playerSessionHandler       *clientPlaySessionHandler

	nopSessionHandler
}

func newBackendPlaySessionHandler(serverConn *serverConnection) (netmc.SessionHandler, error) {
	cpsh, ok := serverConn.player.SessionHandler().(*clientPlaySessionHandler)
	if !ok {
		return nil, errors.New("initializing backendPlaySessionHandler with no backing client play session handler")
	}
	return &backendPlaySessionHandler{
		serverConn: serverConn,
		bungeeCordMessageResponder: bungeeCordMessageResponder(
			serverConn.config().BungeePluginChannelEnabled,
			serverConn.player, serverConn.player.proxy,
		),
		playerSessionHandler: cpsh,
	}, nil
}

func (b *backendPlaySessionHandler) HandlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket() {
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
	case *legacytablist.PlayerListItem:
		b.handleLegacyPlayerListItem(p, pc)
	case *playerinfo.Upsert:
		b.handleUpsertPlayerInfo(p, pc)
	case *playerinfo.Remove:
		b.handleRemovePlayerInfo(p, pc)
	case *packet.ResourcePackRequest:
		b.handleResourcePacketRequest(p)
	case *packet.ServerData:
		b.handleServerData(p)
	case *bossbar.BossBar:
		b.handleBossBar(p, pc)
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

func (b *backendPlaySessionHandler) Activated() {
	b.serverConn.server.players.add(b.serverConn.player)
	if b.proxy().cfg.BungeePluginChannelEnabled {
		serverMc, ok := b.serverConn.ensureConnected()
		if ok {
			protocol := serverMc.Protocol()
			channelsPacket := plugin.ConstructChannelsPacket(protocol, bungeecord.Channel(protocol))
			_ = serverMc.WritePacket(channelsPacket)
		}
	}
}

func (b *backendPlaySessionHandler) Disconnected() {
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
	b.serverConn.pendingPings.Set(p.RandomID, time.Now())
	b.forwardToPlayer(pc, nil) // forward on
}

func (b *backendPlaySessionHandler) handleDisconnect(p *packet.Disconnect) {
	b.serverConn.disconnect()
	b.serverConn.player.handleDisconnect(b.serverConn.server, p, true)
}

func (b *backendPlaySessionHandler) handleBossBar(p *bossbar.BossBar, pc *proto.PacketContext) {
	switch p.Action {
	case bossbar.AddAction:
		b.playerSessionHandler.serverBossBars[p.ID] = struct{}{}
	case bossbar.RemoveAction:
		delete(b.playerSessionHandler.serverBossBars, p.ID)
	}
	b.forwardToPlayer(pc, nil) // forward on
}

func (b *backendPlaySessionHandler) handlePluginMessage(packet *plugin.Message, pc *proto.PacketContext) {
	if b.bungeeCordMessageResponder.Process(packet) {
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

	if b.serverConn.phase().Handle(
		b.serverConn.player,
		b.serverConn,
		b.serverConn.conn(),
		b.serverConn.player,
		packet,
	) {
		return // handled
	}

	id, ok := b.proxy().ChannelRegistrar().FromID(packet.Channel)
	if !ok {
		b.forwardToPlayer(pc, nil)
		return
	}

	clone := make([]byte, len(packet.Data))
	copy(clone, packet.Data)
	event.FireParallel(b.proxy().Event(), &PluginMessageEvent{
		source:     b.serverConn,
		target:     b.serverConn.player,
		identifier: id,
		data:       clone,
		forward:    true,
	}, func(pme *PluginMessageEvent) {
		if pme.Allowed() && b.serverConn.player.Active() {
			b.forwardToPlayer(nil, &plugin.Message{
				Channel: packet.Channel,
				Data:    clone,
			})
		}
	})
}

func (b *backendPlaySessionHandler) handleServerData(p *packet.ServerData) {
	ping := newInitialPing(b.proxy(), b.serverConn.player.Protocol())
	e := &PingEvent{
		inbound: b.serverConn.player,
		ping:    ping,
	}
	event.FireParallel(b.proxy().Event(), e, func(e *PingEvent) {
		if e.ping == nil {
			return
		}
		if !e.Connection().Active() {
			return
		}
		_ = b.serverConn.player.WritePacket(&packet.ServerData{
			Description:        e.Ping().Description,
			Favicon:            e.Ping().Favicon,
			SecureChatEnforced: p.SecureChatEnforced,
		})
	})
}

var sha1HexRegex = regexp.MustCompile(`[0-9a-fA-F]{40}`)

func (b *backendPlaySessionHandler) handleResourcePacketRequest(p *packet.ResourcePackRequest) {
	packInfo := ResourcePackInfo{
		URL:         p.URL,
		ShouldForce: p.Required,
		Prompt:      p.Prompt,
		Origin:      DownstreamServerResourcePackOrigin,
	}

	if p.Hash != "" && sha1HexRegex.MatchString(p.Hash) {
		var err error
		packInfo.Hash, err = hex.DecodeString(p.Hash)
		if err != nil {
			b.serverConn.log.V(1).Error(err, "error decoding resource pack hash")
			return
		}
	}

	e := &ServerResourcePackSendEvent{
		receivedResourcePack: packInfo,
		providedResourcePack: packInfo,
		serverConn:           b.serverConn,
	}
	b.proxy().event.Fire(e)

	if netmc.Closed(b.serverConn.player) {
		return
	}
	if e.Allowed() {
		toSend := e.ProvidedResourcePack()
		if reflect.DeepEqual(toSend, e.ReceivedResourcePack()) {
			toSend.Origin = DownstreamServerResourcePackOrigin
		}

		err := b.serverConn.player.queueResourcePack(toSend)
		if err != nil {
			b.serverConn.log.V(1).Error(err, "error queuing resource pack")
		}
	} else if smc, ok := b.serverConn.ensureConnected(); ok {
		err := smc.WritePacket(&packet.ResourcePackResponse{
			Hash:   p.Hash,
			Status: DeclinedResourcePackResponseStatus,
		})
		if err != nil {
			b.serverConn.log.V(1).Error(err, "error sending resource pack response")
		}
	}
}

func (b *backendPlaySessionHandler) handleLegacyPlayerListItem(p *legacytablist.PlayerListItem, pc *proto.PacketContext) {
	if err := b.serverConn.player.tabList.ProcessLegacy(p); err != nil {
		b.serverConn.log.Error(err, "error processing backend LegacyPlayerListItem packet, ignored")
	}
	b.forwardToPlayer(pc, nil)
}

func (b *backendPlaySessionHandler) handleUpsertPlayerInfo(p *playerinfo.Upsert, pc *proto.PacketContext) {
	if err := b.serverConn.player.tabList.ProcessUpdate(p); err != nil {
		b.serverConn.log.Error(err, "error processing backend UpsertPlayerInfo packet, ignored")
	}
	b.forwardToPlayer(pc, nil)
}

func (b *backendPlaySessionHandler) handleRemovePlayerInfo(p *playerinfo.Remove, pc *proto.PacketContext) {
	b.serverConn.player.tabList.ProcessRemove(p)
	b.forwardToPlayer(pc, nil)
}

func (b *backendPlaySessionHandler) handleAvailableCommands(p *packet.AvailableCommands) {
	rootNode := p.RootNode
	if b.proxy().cfg.AnnounceProxyCommands {
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

	event.FireParallel(b.proxy().Event(),
		&PlayerAvailableCommandsEvent{
			player:   b.serverConn.player,
			rootNode: rootNode,
		}, func(e *PlayerAvailableCommandsEvent) {
			_ = b.serverConn.player.WritePacket(p)
		})
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

var _ netmc.SessionHandler = (*backendPlaySessionHandler)(nil)

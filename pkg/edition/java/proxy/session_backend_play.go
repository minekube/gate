package proxy

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/internal/resourcepack"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"

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
	"go.uber.org/atomic"
)

type backendPlaySessionHandler struct {
	serverConn                 *serverConnection
	bungeeCordMessageResponder bungeecord.MessageResponder
	exceptionTriggered         atomic.Bool
	playerSessionHandler       *clientPlaySessionHandler
	log                        logr.Logger

	nopSessionHandler
}

func newBackendPlaySessionHandler(serverConn *serverConnection) (netmc.SessionHandler, error) {
	activeHandler := serverConn.player.ActiveSessionHandler()
	psh, ok := activeHandler.(*clientPlaySessionHandler)
	if !ok {
		return nil, fmt.Errorf("initializing backendPlaySessionHandler with no backing client play session handler, got %T", activeHandler)
	}
	return &backendPlaySessionHandler{
		serverConn: serverConn,
		bungeeCordMessageResponder: newBungeeCordMessageResponder(
			serverConn.config().BungeePluginChannelEnabled,
			serverConn.player, serverConn.player.proxy,
		),
		playerSessionHandler: psh,
		log:                  serverConn.log.WithName("backendPlaySessionHandler"),
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
	case *config.StartUpdate:
		b.handleStartUpdate(p)
	case *packet.ClientSettings:
		b.handleClientSettings(p)
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
	case *packet.RemoveResourcePack:
		b.handleRemoveResourcePack(p)
	case *packet.ServerData:
		b.handleServerData(p)
	case *bossbar.BossBar:
		b.handleBossBar(p, pc)
	case *packet.BundleDelimiter:
		b.serverConn.player.bundleHandler.ToggleBundleSession()
		b.forwardToPlayer(pc, nil)
	case *packet.Transfer:
		b.handleTransfer(p)
	case *cookie.CookieStore:
		b.handleCookieStore(p)
	case *cookie.CookieRequest:
		b.handleCookieRequest(p)
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

func (b *backendPlaySessionHandler) handleStartUpdate(_ *config.StartUpdate) {
	smc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}
	smc.SetAutoReading(false)
	smc.Reader().SetState(state.Config)
	b.serverConn.player.switchToConfigState()
}

func (b *backendPlaySessionHandler) handleClientSettings(p *packet.ClientSettings) {
	if err := b.serverConn.connection.WritePacket(p); err != nil {
		b.log.V(1).Error(err, "error writing ClientSettings packet to server")
	}
}

func (b *backendPlaySessionHandler) handleBossBar(p *bossbar.BossBar, pc *proto.PacketContext) {
	b.playerSessionHandler.mu.Lock()
	switch p.Action {
	case bossbar.AddAction:
		b.playerSessionHandler.mu.serverBossBars[p.ID] = struct{}{}
	case bossbar.RemoveAction:
		delete(b.playerSessionHandler.mu.serverBossBars, p.ID)
	default:
	}
	b.playerSessionHandler.mu.Unlock()
	b.forwardToPlayer(pc, nil) // forward on
}

func (b *backendPlaySessionHandler) handlePluginMessage(packet *plugin.Message, pc *proto.PacketContext) {
	if b.bungeeCordMessageResponder.Process(packet) {
		return
	}

	// Register and unregister packets are simply forwarded to the server as-is.
	if plugin.IsRegister(packet) || plugin.IsUnregister(packet) {
		if serverMc, ok := b.serverConn.ensureConnected(); ok {
			_ = serverMc.WritePacket(packet)
		}
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
		if pme.Allowed() && b.serverConn.active() {
			b.forwardToPlayer(nil, &plugin.Message{
				Channel: packet.Channel,
				Data:    pme.Data(),
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
			Description:        &chat.ComponentHolder{Component: e.Ping().Description},
			Favicon:            e.Ping().Favicon,
			SecureChatEnforced: p.SecureChatEnforced,
		})
	})
}

func toServerPromptedResourcePack(p *packet.ResourcePackRequest) (*ResourcePackInfo, error) {
	info, err := resourcepack.InfoForRequest(p)
	if err != nil {
		return nil, err
	}
	info.Origin = DownstreamServerResourcePackOrigin
	return info, nil
}

func (b *backendPlaySessionHandler) handleResourcePacketRequest(p *packet.ResourcePackRequest) {
	handleResourcePacketRequest(p, b.serverConn, b.proxy().Event(), b.log)
}

func handleResourcePacketRequest(
	p *packet.ResourcePackRequest,
	serverConn *serverConnection,
	eventMgr event.Manager,
	log logr.Logger,
) {
	err := handleResourcePacketRequest_(p, serverConn, eventMgr, log)
	if err != nil {
		log.V(1).Error(err, "error handling ResourcePackRequest packet from backend server, declining it")
		if smc, ok := serverConn.ensureConnected(); ok {
			_ = smc.WritePacket(&packet.ResourcePackResponse{
				ID:     p.ID,
				Hash:   p.Hash,
				Status: DeclinedResourcePackResponseStatus,
			})
		}
	}
}
func handleResourcePacketRequest_(
	p *packet.ResourcePackRequest,
	serverConn *serverConnection,
	eventMgr event.Manager,
	log logr.Logger,
) (err error) {
	packInfo, err := toServerPromptedResourcePack(p)
	if err != nil {
		return fmt.Errorf("error converting ResourcePackRequest to ResourcePackInfo: %w", err)
	}

	e := newServerResourcePackSendEvent(*packInfo, serverConn)
	eventMgr.Fire(e)

	if netmc.Closed(serverConn.player) {
		return
	}
	if e.Allowed() {
		toSend := e.ProvidedResourcePack()
		modifiedPack := false
		if !reflect.DeepEqual(toSend, e.ReceivedResourcePack()) {
			toSend.Origin = DownstreamServerResourcePackOrigin
			modifiedPack = true
		}
		if serverConn.player.resourcePackHandler.HasPackAppliedByHash(toSend.Hash) {
			// Do not apply a resource pack that has already been applied
			if mcConn := serverConn.conn(); mcConn != nil {
				err = mcConn.WritePacket(&packet.ResourcePackResponse{
					ID:     p.ID,
					Hash:   p.Hash,
					Status: AcceptedResourcePackResponseStatus,
				})
				if err != nil {
					return fmt.Errorf("error sending accepted resource pack response: %w", err)
				}
				if mcConn.Protocol().GreaterEqual(version.Minecraft_1_20_3) {
					err = mcConn.WritePacket(&packet.ResourcePackResponse{
						ID:     p.ID,
						Hash:   p.Hash,
						Status: DownloadedResourcePackResponseStatus,
					})
					if err != nil {
						return fmt.Errorf("error sending downloaded resource pack response: %w", err)
					}
				}
				err = mcConn.WritePacket(&packet.ResourcePackResponse{
					ID:     p.ID,
					Hash:   p.Hash,
					Status: SuccessfulResourcePackResponseStatus,
				})
				if err != nil {
					return fmt.Errorf("error sending successful resource pack response: %w", err)
				}
			}
			if modifiedPack {
				log.Info("A plugin has tried to modify a ResourcePack provided by the backend server " +
					"with a ResourcePack already applied, the applying of the resource pack will be skipped.")
			}
		} else {
			err = serverConn.player.resourcePackHandler.QueueResourcePack(&toSend)
			if err != nil {
				return fmt.Errorf("error queuing resource pack: %w", err)
			}
		}
	} else if smc, ok := serverConn.ensureConnected(); ok {
		err = smc.WritePacket(&packet.ResourcePackResponse{
			ID:     p.ID,
			Hash:   p.Hash,
			Status: DeclinedResourcePackResponseStatus,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *backendPlaySessionHandler) handleRemoveResourcePack(p *packet.RemoveResourcePack) {
	handler := b.serverConn.player.resourcePackHandler
	if p.ID != uuid.Nil {
		handler.Remove(p.ID)
	} else {
		handler.ClearAppliedResourcePacks()
	}
	b.forwardToPlayer(nil, p)
}

func (b *backendPlaySessionHandler) handleTransfer(p *packet.Transfer) {
	handleTransfer(p, b.serverConn.player, b.log, b.proxy().Event())
}
func handleTransfer(p *packet.Transfer, player Player, log logr.Logger, mgr event.Manager) {
	originalAddr, err := p.Addr()
	if err != nil {
		log.Error(err, "error getting address from Transfer packet received from Backend Server in Play State")
		return
	}
	event.FireParallel(mgr, newPreTransferEvent(player, originalAddr), func(e *PreTransferEvent) {
		if e.Allowed() {
			resultedAddr := e.Addr()
			if resultedAddr == nil {
				resultedAddr = originalAddr
			}
			host, port := netutil.HostPort(resultedAddr)
			err = player.WritePacket(&packet.Transfer{
				Host: host,
				Port: int(port),
			})
			if err != nil {
				log.V(1).Error(err, "error sending Transfer packet to player")
			}
		}
	})
}

func (b *backendPlaySessionHandler) handleLegacyPlayerListItem(p *legacytablist.PlayerListItem, pc *proto.PacketContext) {
	if err := b.serverConn.player.tabList.ProcessLegacy(p); err != nil {
		b.log.Error(err, "error processing backend LegacyPlayerListItem packet, ignored")
	}
	b.forwardToPlayer(pc, nil)
}

func (b *backendPlaySessionHandler) handleUpsertPlayerInfo(p *playerinfo.Upsert, pc *proto.PacketContext) {
	if err := b.serverConn.player.tabList.ProcessUpdate(p); err != nil {
		b.log.Error(err, "error processing backend UpsertPlayerInfo packet, ignored")
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

func (b *backendPlaySessionHandler) handleCookieStore(p *cookie.CookieStore) {
	e := newCookieStoreEvent(b.serverConn.player, p.Key, p.Payload)
	b.proxy().event.Fire(e)
	if !e.Allowed() {
		return
	}
	forwardCookieStore(e, b.serverConn.player)
}

func (b *backendPlaySessionHandler) handleCookieRequest(p *cookie.CookieRequest) {
	e := newCookieRequestEvent(b.serverConn.player, p.Key)
	b.proxy().event.Fire(e)
	if !e.Allowed() {
		return
	}
	forwardCookieRequest(e, b.serverConn.player)
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

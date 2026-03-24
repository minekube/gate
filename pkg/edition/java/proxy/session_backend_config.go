package proxy

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// backendConfigSessionHandler is a special session handler that catches "last minute" disconnects.
// This version is to accommodate 1.20.2+ switching. It handles the transition of a player between servers in a proxy setup.
// This is a complex process that involves multiple stages and can be interrupted by various events, such as plugin messages or resource pack requests.
type backendConfigSessionHandler struct {
	serverConn          *serverConnection
	requestCtx          *connRequestCxt
	state               backendConfigSessionState
	resourcePackToApply *ResourcePackInfo
	log                 logr.Logger

	nopSessionHandler
}

// newBackendConfigSessionHandler creates a new backendConfigSessionHandler.
func newBackendConfigSessionHandler(
	serverConn *serverConnection,
	requestCtx *connRequestCxt,
) (netmc.SessionHandler, error) {
	return &backendConfigSessionHandler{
		serverConn: serverConn,
		state:      backendConfigSessionStateStart,
		requestCtx: requestCtx,
		log:        serverConn.log.WithName("backendConfigSessionHandler"),
	}, nil
}

type backendConfigSessionState uint8

const (
	backendConfigSessionStateStart backendConfigSessionState = iota
	backendConfigSessionStateNegotiating
	backendConfigSessionStatePluginMessageInterrupt
	backendConfigSessionStateResourcePackInterrupt
	backendConfigSessionStateComplete
)

// HandlePacket handles incoming packets. It checks if the packet is known and if the connection should handle it.
// It then switches on the type of the packet and calls the appropriate handler method.
func (b *backendConfigSessionHandler) HandlePacket(pc *proto.PacketContext) {
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
		b.handleKeepAlive(p)
	case *config.StartUpdate:
		b.forwardToServer(pc, nil)
	case *packet.ResourcePackRequest:
		b.handleResourcePackRequest(p)
	case *packet.RemoveResourcePack:
		b.handleRemoveResourcePackRequest(p)
	case *config.FinishedUpdate:
		b.handleFinishedUpdate(p)
	case *config.TagsUpdate:
		b.forwardToPlayer(pc, nil)
	case *config.RegistrySync:
		b.forwardToPlayer(pc, nil)
	case *plugin.Message:
		b.handlePluginMessage(pc, p)
	case *packet.Disconnect:
		b.serverConn.disconnect()
		// If the player receives a DisconnectPacket without a connection to a server in progress,
		// it means that the backend server has kicked the player during reconfiguration
		if b.serverConn.player.connectionInFlight() != nil {
			result := disconnectResultForPacket(b.log.V(1), p,
				b.serverConn.player.Protocol(), b.serverConn.server, true,
			)
			b.requestCtx.result(result, nil)
		} else {
			b.serverConn.player.handleDisconnect(b.serverConn.server, p, true)
		}
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

// shouldHandle checks if the server connection is active. If it's not, it disconnects the server connection and returns false.
func (b *backendConfigSessionHandler) shouldHandle() bool {
	if b.serverConn.active() {
		return true
	}
	// Obsolete connection
	b.serverConn.disconnect()
	return false
}

// Activated is called when the session handler is activated.
func (b *backendConfigSessionHandler) Activated() {
	player := b.serverConn.player
	if player.Protocol() == version.Minecraft_1_20_2.Protocol {
		b.resourcePackToApply = player.resourcePackHandler.FirstAppliedPack()
		player.resourcePackHandler.ClearAppliedResourcePacks()
	}
}

// Disconnected is called when the session handler is disconnected.
func (b *backendConfigSessionHandler) Disconnected() {
	b.requestCtx.result(nil, errors.New("unexpectedly disconnected from remote server"))
}

func (b *backendConfigSessionHandler) handleResourcePackRequest(p *packet.ResourcePackRequest) {
	handleResourcePacketRequest(p, b.serverConn, b.proxy().Event(), b.log)
}

func (b *backendConfigSessionHandler) handleRemoveResourcePackRequest(p *packet.RemoveResourcePack) {
	player := b.serverConn.player

	// TODO add ServerResourcePackRemoveEvent
	handler := player.resourcePackHandler
	if p.ID != uuid.Nil {
		handler.Remove(p.ID)
	} else {
		handler.ClearAppliedResourcePacks()
	}
	_ = player.WritePacket(p)
}

func (b *backendConfigSessionHandler) handleFinishedUpdate(p *config.FinishedUpdate) {
	smc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}
	player := b.serverConn.player

	activehandler := player.ActiveSessionHandler()
	configHandler, ok := activehandler.(*clientConfigSessionHandler)
	if !ok {
		err := fmt.Errorf("expected client config session handler, got %T", activehandler)
		b.log.Error(err, "error handling finished update packet")
		b.serverConn.disconnect()
		b.requestCtx.result(nil, err)
		return
	}

	smc.Reader().SetState(state.Play)
	configHandler.handleBackendFinishUpdate(b.serverConn, p).ThenAccept(func(any) {
		err := smc.WritePacket(&config.FinishedUpdate{})
		if err != nil {
			b.log.Error(err, "error writing finished update packet")
			b.serverConn.disconnect()
			b.requestCtx.result(nil, fmt.Errorf("error writing finished update packet: %w", err))
			return
		}

		if b.serverConn == player.connectedServer() {
			if !smc.SwitchSessionHandler(state.Play) {
				err := errors.New("failed to switch session handler")
				b.log.Error(err, "expected to switch session handler to play state")
				b.serverConn.disconnect()
				b.requestCtx.result(nil, err)
				return
			}

			header, footer := player.tabList.HeaderFooter()
			err := tablist.SendHeaderFooter(player, header, footer)
			if err != nil {
				b.log.Error(err, "error sending tab list header/footer")
				return
			}

			// The client cleared the tab list.
			//  TODO: Restore changes done via TabList API
			err = player.tabList.RemoveAll()
			if err != nil {
				b.log.Error(err, "error removing all tab list entries")
				return
			}
		} else {
			smc.SetActiveSessionHandler(state.Play,
				newBackendTransitionSessionHandler(
					b.serverConn, b.requestCtx, b.proxy(),
				),
			)
		}

		if player.resourcePackHandler.FirstAppliedPack() == nil && b.resourcePackToApply != nil {
			_ = player.resourcePackHandler.QueueResourcePack(b.resourcePackToApply)
		}
	})
}

func (b *backendConfigSessionHandler) handleTransfer(p *packet.Transfer) {
	handleTransfer(p, b.serverConn.player, b.log, b.proxy().Event())
}

func (b *backendConfigSessionHandler) handlePluginMessage(pc *proto.PacketContext, p *plugin.Message) {
	if plugin.McBrand(p) {
		_ = b.serverConn.player.WritePacket(plugin.RewriteMinecraftBrand(p,
			b.serverConn.player.Protocol()))
	} else {
		bytes := pc.Payload
		id, ok := b.proxy().ChannelRegistrar().FromID(p.Channel)
		if !ok {
			b.forwardToPlayer(pc, nil)
			return
		}

		// Handling this stuff async means that we should probably pause
		// the connection while we toss this off into another pool
		b.serverConn.connection.SetAutoReading(false)
		event.FireParallel(b.proxy().Event(), &PluginMessageEvent{
			source:     b.serverConn,
			target:     b.serverConn.player,
			identifier: id,
			data:       bytes,
		}, func(pme *PluginMessageEvent) {
			if pme.Allowed() && b.serverConn.active() {
				b.forwardToPlayer(pc, &plugin.Message{
					Channel: p.Channel,
					Data:    pme.Data(),
				})
			}
			b.serverConn.connection.SetAutoReading(true)
		})
	}
}

func (b *backendConfigSessionHandler) handleKeepAlive(p *packet.KeepAlive) {
	b.serverConn.pendingPings.Set(p.RandomID, time.Now())
	_ = b.serverConn.player.WritePacket(p)
}

func (b *backendConfigSessionHandler) handleCookieRequest(p *cookie.CookieRequest) {
	e := newCookieRequestEvent(b.serverConn.player, p.Key)
	b.proxy().event.Fire(e)
	if !e.Allowed() {
		return
	}
	forwardCookieRequest(e, b.serverConn.player)
}

func forwardCookieRequest(e *CookieRequestEvent, conn netmc.MinecraftConn) {
	key := e.Key()
	if key == nil {
		key = e.OriginalKey()
	}
	_ = conn.WritePacket(&cookie.CookieRequest{
		Key: key,
	})
}

func (b *backendConfigSessionHandler) handleCookieStore(p *cookie.CookieStore) {
	e := newCookieStoreEvent(b.serverConn.player, p.Key, p.Payload)
	b.proxy().event.Fire(e)
	if !e.Allowed() {
		return
	}
	forwardCookieStore(e, b.serverConn.player)
}

func forwardCookieStore(e *CookieStoreEvent, conn netmc.MinecraftConn) {
	key := e.Key()
	if key == nil {
		key = e.OriginalKey()
	}
	payload := e.Payload()
	if payload == nil {
		payload = e.OriginalPayload()
	}
	_ = conn.WritePacket(&cookie.CookieStore{
		Key:     key,
		Payload: payload,
	})
}

// forwardToPlayer forwards packets to the player. It prefers PacketContext over Packet.
// Since we already have the packet's payload we can simply forward it on,
// instead of encoding a Packet again each time. This increases throughput & decreases CPU and memory usage.
func (b *backendConfigSessionHandler) forwardToPlayer(packetContext *proto.PacketContext, packet proto.Packet) {
	if packetContext == nil {
		_ = b.serverConn.player.WritePacket(packet)
		return
	}
	_ = b.serverConn.player.Write(packetContext.Payload)
}

// forwardToServer forwards packets to the server. It prefers PacketContext over Packet.
func (b *backendConfigSessionHandler) forwardToServer(packetContext *proto.PacketContext, packet proto.Packet) {
	if packetContext == nil {
		_ = b.serverConn.connection.WritePacket(packet)
		return
	}
	_ = b.serverConn.connection.Write(packetContext.Payload)
}

// proxy returns the Proxy of the player.
func (b *backendConfigSessionHandler) proxy() *Proxy {
	return b.serverConn.player.proxy
}

var _ netmc.SessionHandler = (*backendConfigSessionHandler)(nil)

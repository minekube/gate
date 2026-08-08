package proxy

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/gammazero/deque"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/edition/java/proxy/internal/resourcepack"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/future"
)

type clientConfigSessionHandler struct {
	player *connectedPlayer
	log    logr.Logger

	configSwitchDone future.Future[any]

	mu struct {
		sync.Mutex
		pluginMessages           deque.Deque[*plugin.Message]
		pluginMessagesBytes      int
		pluginMessagesOverflowed bool
		brandChannel             string
		brandForwardedServer     *serverConnection
		readyServer              *serverConnection
	}

	nopSessionHandler
}

func newClientConfigSessionHandler(
	player *connectedPlayer,
) *clientConfigSessionHandler {
	return &clientConfigSessionHandler{
		player: player,
		log:    player.log.WithName("clientConfigSessionHandler"),
	}
}

// Disconnected is called when the player disconnects.
func (h *clientConfigSessionHandler) Disconnected() {
	h.player.teardown()
}

func (h *clientConfigSessionHandler) HandlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket() {
		forwardToServer(pc, h.player)
		return
	}
	switch p := pc.Packet.(type) {
	case *packet.KeepAlive:
		forwardKeepAlive(p, h.player)
	case *packet.ClientSettings:
		h.player.setClientSettings(p)
	case *config.CodeOfConductAcceptPacket:
		h.markCodeOfConductAccepted()
		forwardToServer(pc, h.player)
	case *packet.ResourcePackResponse:
		if !handleResourcePackResponse(p, h.player.resourcePackHandler, h.log) {
			forwardToServer(pc, h.player)
		}
	case *config.FinishedUpdate:
		h.player.SetActiveSessionHandler(state.Play, newClientPlaySessionHandler(h.player))
		h.configSwitchDone.Complete(nil)
	case *plugin.Message:
		h.handlePluginMessage(p)
	case *packet.PingIdentify:
		if s := h.player.connectionInFlight(); s != nil {
			smc, ok := s.ensureConnected()
			if ok {
				_ = smc.WritePacket(p)
			}
		}
	case *config.KnownPacks:
		h.handleKnownPacks(p, pc)
	case *cookie.CookieResponse:
		h.handleCookieResponse(p)
	default:
		forwardToServer(pc, h.player)
	}
}

// handleBackendFinishUpdate handles the backend finishing the config stage.
func (h *clientConfigSessionHandler) handleBackendFinishUpdate(serverConn *serverConnection, p *config.FinishedUpdate) *future.Future[any] {
	_, ok := serverConn.ensureConnected()
	if !ok {
		return nil
	}
	brand := serverConn.player.ClientBrand()
	if brand != "" {
		h.writeBrandPacketTo(serverConn, brand)
	}

	if err := h.player.WritePacket(p); err != nil {
		return nil
	}
	h.player.SetOutboundState(state.Play)

	return &h.configSwitchDone
}

func (h *clientConfigSessionHandler) markCodeOfConductAccepted() {
	if serverConn := h.player.connectionInFlightOrConnectedServer(); serverConn != nil {
		if smc, ok := serverConn.ensureConnected(); ok {
			if backendConfig, ok := smc.ActiveSessionHandler().(*backendConfigSessionHandler); ok {
				backendConfig.releaseCodeOfConductHold()
			}
		}
	}
}

func handleResourcePackResponse(p *packet.ResourcePackResponse, handler resourcepack.Handler, log logr.Logger) bool {
	handled, err := handler.OnResourcePackResponse(
		resourcepack.BundleForResponse(p))
	if err != nil {
		log.V(1).Error(err, "Error handling resource pack response")
		return true
	}
	return handled
}

func (h *clientConfigSessionHandler) handlePluginMessage(p *plugin.Message) {
	if plugin.McBrand(p) {
		brand := plugin.ReadBrandMessage(p.Data)
		h.player.setClientBrand(brand)
		readyServer := h.setBrandChannel(p.Channel)
		h.event().FireParallel(&PlayerClientBrandEvent{
			player: h.player,
			brand:  brand,
		})
		// Client sends `minecraft:brand` packet immediately after Login,
		// but at this time the backend server may not be ready
		if readyServer != nil {
			h.writeBrandPacketTo(readyServer, brand)
		}
		return
	} else if bungeecord.IsBungeeCordMessage(p) {
		return
	} else if h.enqueuePluginMessage(h.player.connectionInFlightOrConnectedServer(), p) {
		return
	} else {
		serverConn := h.player.connectionInFlightOrConnectedServer()
		if serverConn == nil {
			return
		}
		id, ok := h.player.proxy.ChannelRegistrar().FromID(p.Channel)
		if !ok {
			smc, ok := serverConn.ensureConnected()
			if ok {
				_ = smc.WritePacket(p)
			}
			return
		}

		// Handling this stuff async means that we should probably pause
		// the connection while we toss this off into another pool
		serverConn.player.SetAutoReading(false)
		event.FireParallel(h.event(), &PluginMessageEvent{
			source:     serverConn,
			target:     h.player,
			identifier: id,
			data:       p.Data,
		}, func(pme *PluginMessageEvent) {
			if pme.Allowed() && serverConn.active() {
				smc, ok := serverConn.ensureConnected()
				if ok {
					_ = smc.WritePacket(&plugin.Message{
						Channel: p.Channel,
						Data:    pme.data,
					})
				}
			}
			serverConn.player.SetAutoReading(true)
		})
	}
}

func (h *clientConfigSessionHandler) setBrandChannel(channel string) *serverConnection {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mu.brandChannel = channel
	return h.mu.readyServer
}

func (h *clientConfigSessionHandler) brandPacket(brand string) *plugin.Message {
	h.mu.Lock()
	channel := h.mu.brandChannel
	h.mu.Unlock()
	if channel == "" {
		return nil
	}
	buf := new(bytes.Buffer)
	_ = util.WriteString(buf, brand)
	return &plugin.Message{
		Channel: channel,
		Data:    buf.Bytes(),
	}
}

func (h *clientConfigSessionHandler) writeBrandPacketTo(serverConn *serverConnection, brand string) {
	h.mu.Lock()
	if h.mu.brandForwardedServer == serverConn {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	brandPacket := h.brandPacket(brand)
	if brandPacket == nil {
		return
	}
	smc, ok := serverConn.ensureConnected()
	if !ok {
		return
	}
	if err := smc.WritePacket(brandPacket); err != nil {
		return
	}

	h.mu.Lock()
	h.mu.brandForwardedServer = serverConn
	h.mu.Unlock()
}

// enqueuePluginMessage returns true when the message was handled by the queue
// path, including overflow rejection. It returns false only once the backend is
// ready for direct config plugin messages.
func (h *clientConfigSessionHandler) enqueuePluginMessage(target *serverConnection, msg *plugin.Message) bool {
	h.mu.Lock()
	if target != nil && h.mu.readyServer == target {
		h.mu.Unlock()
		return false
	}
	if h.mu.pluginMessagesOverflowed {
		h.mu.Unlock()
		return true
	}
	newBytes := h.mu.pluginMessagesBytes + len(msg.Data)
	newCount := h.mu.pluginMessages.Len() + 1
	if newBytes > maxQueuedLoginPluginMessageBytes || newCount > maxQueuedLoginPluginMessages {
		h.mu.pluginMessagesOverflowed = true
		h.mu.pluginMessages.Clear()
		h.mu.pluginMessagesBytes = 0
		h.mu.Unlock()
		h.log.Info("disconnecting player: pre-backend config plugin message queue exceeded its limits",
			"messages", newCount, "bytes", newBytes)
		h.player.Disconnect(&component.Text{
			Content: "Too many plugin messages were sent before joining a server",
		})
		return true
	}
	h.mu.pluginMessages.PushBack(&plugin.Message{
		Channel: msg.Channel,
		Data:    append([]byte(nil), msg.Data...),
	})
	h.mu.pluginMessagesBytes = newBytes
	h.mu.Unlock()
	return true
}

func (h *clientConfigSessionHandler) flushQueuedPluginMessagesTo(serverConn *serverConnection) error {
	smc, ok := serverConn.ensureConnected()
	if !ok {
		return netmc.ErrClosedConn
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.mu.readyServer == serverConn {
		return nil
	}

	h.mu.pluginMessagesBytes = 0
	n := h.mu.pluginMessages.Len()
	msgs := make([]*plugin.Message, 0, n)
	for h.mu.pluginMessages.Len() != 0 {
		msgs = append(msgs, h.mu.pluginMessages.PopFront())
	}
	for _, pm := range msgs {
		if err := smc.BufferPacket(pm); err != nil {
			return fmt.Errorf("error buffering queued config plugin message %q: %w", pm.Channel, err)
		}
	}
	if n != 0 {
		if err := smc.Flush(); err != nil {
			return err
		}
	}
	h.mu.readyServer = serverConn
	return nil
}

func (h *clientConfigSessionHandler) handleKnownPacks(p *config.KnownPacks, pc *proto.PacketContext) {
	serverConn := h.player.connectionInFlightOrConnectedServer()
	if serverConn == nil {
		return
	}
	smc, ok := serverConn.ensureConnected()
	if ok {
		_ = smc.WritePacket(p)
	}
}

func (h *clientConfigSessionHandler) event() event.Manager {
	return h.player.proxy.Event()
}

func (h *clientConfigSessionHandler) handleCookieResponse(p *cookie.CookieResponse) {
	e := newCookieReceiveEvent(h.player, p.Key, p.Payload)
	h.event().Fire(e)
	if !e.Allowed() {
		return
	}
	smc, ok := h.player.connectionInFlightOrConnectedServer().ensureConnected()
	if !ok {
		return
	}
	forwardCookieReceive(e, smc)
}

func forwardCookieReceive(e *CookieReceiveEvent, conn netmc.MinecraftConn) {
	key := e.Key()
	if key == nil {
		key = e.OriginalKey()
	}
	payload := e.Payload()
	if payload == nil {
		payload = e.OriginalPayload()
	}
	_ = conn.WritePacket(&cookie.CookieResponse{
		Key:     key,
		Payload: payload,
	})
}

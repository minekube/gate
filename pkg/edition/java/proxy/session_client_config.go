package proxy

import (
	"bytes"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/internal/resourcepack"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/future"
)

type clientConfigSessionHandler struct {
	player *connectedPlayer
	log    logr.Logger

	brandChannel string

	configSwitchDone future.Future[any]

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
	default:
		forwardToServer(pc, h.player)
	}
}

// handleBackendFinishUpdate handles the backend finishing the config stage.
func (h *clientConfigSessionHandler) handleBackendFinishUpdate(serverConn *serverConnection, p *config.FinishedUpdate) *future.Future[any] {
	smc, ok := serverConn.ensureConnected()
	if !ok {
		return nil
	}
	brand := serverConn.player.ClientBrand()
	if brand == "" && h.brandChannel != "" {
		buf := new(bytes.Buffer)
		_ = util.WriteString(buf, brand)

		brandPacket := &plugin.Message{
			Channel: h.brandChannel,
			Data:    buf.Bytes(),
		}
		_ = smc.WritePacket(brandPacket)
	}

	if err := h.player.WritePacket(p); err != nil {
		return nil
	}
	h.player.Writer().SetState(state.Play)

	return &h.configSwitchDone
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
	serverConn := h.player.connectionInFlight()
	if serverConn == nil {
		return
	}

	if plugin.McBrand(p) {
		brand := plugin.ReadBrandMessage(p.Data)
		h.brandChannel = p.Channel
		h.event().FireParallel(&PlayerClientBrandEvent{
			player: h.player,
			brand:  brand,
		})
		// Client sends `minecraft:brand` packet immediately after Login,
		// but at this time the backend server may not be ready
	} else {
		smc, ok := serverConn.ensureConnected()
		if ok {
			_ = smc.WritePacket(p)
		}
	}
}

func (h *clientConfigSessionHandler) handleKnownPacks(p *config.KnownPacks, pc *proto.PacketContext) {
	smc, ok := h.player.connectionInFlightOrConnectedServer().ensureConnected()
	if ok {
		_ = smc.WritePacket(p)
	}
}

func (h *clientConfigSessionHandler) event() event.Manager {
	return h.player.proxy.Event()
}

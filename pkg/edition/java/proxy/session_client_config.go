package proxy

import (
	"bytes"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/future"
)

type clientConfigSessionHandler struct {
	player *connectedPlayer

	brandChannel string

	configSwitchDone future.Future[any]

	nopSessionHandler
}

func newClientConfigSessionHandler(
	player *connectedPlayer,
) *clientConfigSessionHandler {
	return &clientConfigSessionHandler{
		player: player,
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
		handleKeepAlive(p, h.player)
	case *packet.ClientSettings:
		h.player.setClientSettings(p)
	case *packet.ResourcePackResponse:
		h.handleResourcePackResponse(p)
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

	if err := smc.WritePacket(p); err != nil {
		return nil
	}
	smc.Writer().SetState(state.Play)

	return &h.configSwitchDone
}

func (h *clientConfigSessionHandler) handleResourcePackResponse(p *packet.ResourcePackResponse) {
	if serverConn := h.player.connectionInFlight(); serverConn != nil {
		smc, ok := serverConn.ensureConnected()
		if ok {
			_ = smc.WritePacket(p)
		}
	}
	if !h.player.onResourcePackResponse(p.Status) {
		_ = h.player.WritePacket(p)
	}
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

func (h *clientConfigSessionHandler) event() event.Manager {
	return h.player.proxy.Event()
}

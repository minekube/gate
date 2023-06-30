package proxy

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

type statusSessionHandler struct {
	*sessionHandlerDeps

	conn    netmc.MinecraftConn
	inbound Inbound
	log     logr.Logger

	resolvePingResponse  pingResolveFunc // used in lite mode
	fallbackPingResponse pingFallbackFunc

	receivedRequest bool

	nopSessionHandler
}

type pingResolveFunc func(log logr.Logger, statusRequestCtx *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error)
type pingFallbackFunc func(log logr.Logger, statusRequestCtx *proto.PacketContext) (logr.Logger, *ping.ServerPing, error)

func newStatusSessionHandler(
	conn netmc.MinecraftConn,
	inbound Inbound,
	sessionHandlerDeps *sessionHandlerDeps,
	pingResolveFunc pingResolveFunc,
	fallbackPingResponse pingFallbackFunc,
) netmc.SessionHandler {
	return &statusSessionHandler{
		sessionHandlerDeps: sessionHandlerDeps,
		conn:               conn,
		inbound:            inbound,
		log: logr.FromContextOrDiscard(conn.Context()).WithName("statusSession").WithValues(
			"inbound", inbound,
			"protocol", conn.Protocol()),
		resolvePingResponse:  pingResolveFunc,
		fallbackPingResponse: fallbackPingResponse,
	}
}

func (h *statusSessionHandler) Activated() {
	cfg := h.config()
	var log logr.Logger
	if cfg.Status.LogPingRequests || cfg.Debug {
		log = h.log
	} else {
		log = h.log.V(1)
	}
	log.Info("got server list status request")
}

func (h *statusSessionHandler) HandlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket() {
		// What even is going on? ;D
		_ = h.conn.Close()
		return
	}

	switch pc.Packet.(type) {
	case *packet.StatusRequest:
		h.handleStatusRequest(pc)
	case *packet.StatusPing:
		h.handleStatusPing(pc)
	// TODO add LegacyPing
	default:
		// unexpected packet, simply close
		_ = h.conn.Close()
	}
}

var versionName = fmt.Sprintf("Gate %s", version.SupportedVersionsString)

func newInitialPing(p *Proxy, protocol proto.Protocol) *ping.ServerPing {
	if !version.Protocol(protocol).Supported() {
		protocol = version.MaximumVersion.Protocol
	}
	var modInfo *modinfo.ModInfo
	if p.config().AnnounceForge {
		modInfo = modinfo.Default
	}
	return &ping.ServerPing{
		Version: ping.Version{
			Protocol: protocol,
			Name:     versionName,
		},
		Players: &ping.Players{
			Online: p.PlayerCount(),
			Max:    p.cfg.Status.ShowMaxPlayers,
		},
		Description: p.motd,
		Favicon:     p.favicon,
		ModInfo:     modInfo,
	}
}

func (h *statusSessionHandler) handleStatusRequest(pc *proto.PacketContext) {
	if h.receivedRequest {
		// Already sent response
		_ = h.conn.Close()
		return
	}
	h.receivedRequest = true

	e := &PingEvent{
		inbound: h.inbound,
	}

	e.ping = h.statusResponse(pc)
	if e.ping == nil {
		return
	}

	h.eventMgr.Fire(e)

	if e.ping == nil {
		_ = h.conn.Close()
		h.log.V(1).Info("ping response was set to nil by an event handler, no response is sent")
		return
	}
	if !h.inbound.Active() {
		return
	}

	response, err := json.Marshal(e.ping)
	if err != nil {
		_ = h.conn.Close()
		h.log.Error(err, "error marshaling ping response to json")
		return
	}
	_ = h.conn.WritePacket(&packet.StatusResponse{
		Status: string(response),
	})
}

func (h *statusSessionHandler) statusResponse(pc *proto.PacketContext) *ping.ServerPing {
	hasPingEventSubscriber := h.eventMgr.HasSubscriber((*PingEvent)(nil))
	if h.resolvePingResponse == nil {
		return newInitialPing(h.proxy, h.conn.Protocol())
	}
	log, pingResponse, err := h.resolvePingResponse(h.log, pc)
	if err != nil && h.fallbackPingResponse == nil {
		errs.V(log, err).Info("could not resolve ping", "error", err)
		_ = h.conn.Close()
		return nil
	}
	if err == nil {
		if !hasPingEventSubscriber {
			_ = h.conn.WritePacket(pingResponse)
			return nil
		}
		ping := new(ping.ServerPing)
		if err = json.Unmarshal([]byte(pingResponse.Status), ping); err != nil {
			h.log.V(1).Error(err, "failed to unmarshal status response")
			_ = h.conn.Close()
			return nil
		}
		return ping
	}
	log, fallbackPing, err := h.fallbackPingResponse(h.log, pc)
	if err != nil {
		errs.V(log, err).Info("could not use fallback ping", "error", err)
		_ = h.conn.Close()
		return nil
	}
	if !hasPingEventSubscriber {
		fallbackBytes, err := json.Marshal(fallbackPing)
		if err != nil {
			_ = h.conn.Close()
			h.log.Error(err, "error marshaling fallback response to json")
			return nil
		}
		_ = h.conn.WritePacket(&packet.StatusResponse{
			Status: string(fallbackBytes),
		})
		return nil
	}
	return fallbackPing
}

func (h *statusSessionHandler) handleStatusPing(p *proto.PacketContext) {
	// Just return again and close
	defer h.conn.Close()
	if err := h.conn.Write(p.Payload); err != nil {
		h.log.V(1).Info("error writing StatusPing response", "error", err)
	}
}

package proxy

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/config"
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

	conn                netmc.MinecraftConn
	inbound             Inbound
	log                 logr.Logger
	resolvePingResponse pingResolveFunc // used in lite mode

	receivedRequest bool

	nopSessionHandler
}

type pingResolveFunc func(log logr.Logger, statusRequestCtx *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error)

func newStatusSessionHandler(
	conn netmc.MinecraftConn,
	inbound Inbound,
	sessionHandlerDeps *sessionHandlerDeps,
	pingResolveFunc pingResolveFunc,
) netmc.SessionHandler {
	return &statusSessionHandler{
		sessionHandlerDeps: sessionHandlerDeps,
		conn:               conn,
		inbound:            inbound,
		log: logr.FromContextOrDiscard(conn.Context()).WithName("statusSession").WithValues(
			"inbound", inbound,
			"protocol", conn.Protocol()),
		resolvePingResponse: pingResolveFunc,
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
	if p.cfg.AnnounceForge {
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
		Description: config.StrToTextComponent(p.cfg.Status.Motd).T(),
		Favicon:     p.cfg.Status.Favicon,
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

	log := h.log
	if h.resolvePingResponse == nil {
		e.ping = newInitialPing(h.proxy, pc.Protocol)
	} else {
		var err error
		var res *packet.StatusResponse
		log, res, err = h.resolvePingResponse(h.log, pc)
		if err != nil {
			errs.V(log, err).Info("could not resolve ping", "error", err)
			_ = h.conn.Close()
			return
		}
		if !h.eventMgr.HasSubscriber(e) {
			// Fast path: No event handler, just send response
			_ = h.conn.WritePacket(res)
			return
		}
		// Need to unmarshal status response to ping struct for event handlers
		e.ping = new(ping.ServerPing)
		if err = json.Unmarshal([]byte(res.Status), e.ping); err != nil {
			h.log.V(1).Error(err, "failed to unmarshal status response")
			_ = h.conn.Close()
			return
		}
	}

	h.eventMgr.Fire(e)

	if e.ping == nil {
		_ = h.conn.Close()
		log.V(1).Info("ping response was set to nil by an event handler, no response is sent")
		return
	}
	if !h.inbound.Active() {
		return
	}

	response, err := json.Marshal(e.ping)
	if err != nil {
		_ = h.conn.Close()
		log.Error(err, "error marshaling ping response to json")
		return
	}
	_ = h.conn.WritePacket(&packet.StatusResponse{
		Status: string(response),
	})
}

func (h *statusSessionHandler) handleStatusPing(p *proto.PacketContext) {
	// Just return again and close
	defer h.conn.Close()
	if err := h.conn.Write(p.Payload); err != nil {
		h.log.V(1).Info("error writing StatusPing response", "error", err)
	}
}

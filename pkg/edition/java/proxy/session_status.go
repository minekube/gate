package proxy

import (
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/modinfo"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type statusSessionHandler struct {
	*sessionHandlerDeps

	conn    netmc.MinecraftConn
	inbound Inbound
	log     logr.Logger

	receivedRequest bool

	nopSessionHandler
}

func newStatusSessionHandler(
	conn netmc.MinecraftConn,
	inbound Inbound,
	sessionHandlerDeps *sessionHandlerDeps,
) netmc.SessionHandler {
	return &statusSessionHandler{
		sessionHandlerDeps: sessionHandlerDeps,
		conn:               conn,
		inbound:            inbound,
		log: logr.FromContextOrDiscard(conn.Context()).WithName("statusSession").WithValues(
			"inbound", inbound,
			"protocol", conn.Protocol()),
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
	if !pc.KnownPacket {
		// What even is going on? ;D
		_ = h.conn.Close()
		return
	}

	switch p := pc.Packet.(type) {
	case *packet.StatusRequest:
		h.handleStatusRequest()
	case *packet.StatusPing:
		h.handleStatusPing(p)
	default:
		// unexpected packet, simply close
		_ = h.conn.Close()
	}
}

var versionName = fmt.Sprintf("Gate %s", version.SupportedVersionsString)

func newInitialPing(p *Proxy, protocol proto.Protocol) *ping.ServerPing {
	shownVersion := protocol
	if !version.Protocol(protocol).Supported() {
		shownVersion = version.MaximumVersion.Protocol
	}
	var modInfo *modinfo.ModInfo
	if p.config().AnnounceForge {
		modInfo = modinfo.Default
	}
	return &ping.ServerPing{
		Version: ping.Version{
			Protocol: shownVersion,
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

func (h *statusSessionHandler) handleStatusRequest() {
	if h.receivedRequest {
		// Already sent response
		_ = h.conn.Close()
		return
	}
	h.receivedRequest = true

	e := &PingEvent{
		inbound: h.inbound,
		ping:    newInitialPing(h.proxy, h.conn.Protocol()),
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

func (h *statusSessionHandler) handleStatusPing(p *packet.StatusPing) {
	// Just return again and close
	defer h.conn.Close()
	if err := h.conn.WritePacket(p); err != nil {
		h.log.V(1).Info("error writing StatusPing response", "err", err)
	}
}

package proxy

import (
	"encoding/json"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/logr"
)

type statusSessionHandler struct {
	conn    *minecraftConn
	inbound Inbound
	log     logr.Logger

	receivedRequest bool

	nopSessionHandler
}

func newStatusSessionHandler(conn *minecraftConn, inbound Inbound) sessionHandler {
	return &statusSessionHandler{conn: conn, inbound: inbound,
		log: conn.log.WithName("statusSession").WithValues(
			"inbound", inbound,
			"protocol", conn.protocol)}
}

func (h *statusSessionHandler) activated() {
	cfg := h.conn.proxy.Config()
	if cfg.Status.LogPingRequests || cfg.Debug {
		h.log.Info("Got server list status request")
	}
}

func (h *statusSessionHandler) handlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket {
		// What even is going on? ;D
		_ = h.conn.close()
		return
	}

	switch p := pc.Packet.(type) {
	case *packet.StatusRequest:
		h.handleStatusRequest()
	case *packet.StatusPing:
		h.handleStatusPing(p)
	default:
		// unexpected packet, simply close
		_ = h.conn.close()
	}
}

var versionName = fmt.Sprintf("Gate %s", version.SupportedVersionsString)

func (h *statusSessionHandler) newInitialPing() *ping.ServerPing {
	shownVersion := h.conn.Protocol()
	if !version.Protocol(h.conn.Protocol()).Supported() {
		shownVersion = version.MaximumVersion.Protocol
	}
	return &ping.ServerPing{
		Version: ping.Version{
			Protocol: shownVersion,
			Name:     versionName,
		},
		Players: &ping.Players{
			Online: h.proxy().PlayerCount(),
			Max:    h.proxy().config.Status.ShowMaxPlayers,
		},
		Description: h.proxy().motd,
		Favicon:     h.proxy().favicon,
	}
}

func (h *statusSessionHandler) handleStatusRequest() {
	if h.receivedRequest {
		// Already sent response
		_ = h.conn.close()
		return
	}
	h.receivedRequest = true

	e := &PingEvent{
		inbound: h.inbound,
		ping:    h.newInitialPing(),
	}
	h.proxy().event.Fire(e)

	if e.ping == nil {
		_ = h.conn.close()
		h.log.V(1).Info("Ping response was set to nil by an event handler, no response is sent")
		return
	}
	if !h.inbound.Active() {
		return
	}

	response, err := json.Marshal(e.ping)
	if err != nil {
		_ = h.conn.close()
		h.log.Error(err, "Error marshaling ping response to json")
		return
	}
	_ = h.conn.WritePacket(&packet.StatusResponse{
		Status: string(response),
	})
}

func (h *statusSessionHandler) handleStatusPing(p *packet.StatusPing) {
	// Just return again and close
	defer h.conn.close()
	if err := h.conn.WritePacket(p); err != nil {
		h.log.V(1).Info("Error writing StatusPing response", "err", err)
	}
}

func (h *statusSessionHandler) proxy() *Proxy {
	return h.conn.proxy
}

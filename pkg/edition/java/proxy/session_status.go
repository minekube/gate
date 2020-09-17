package proxy

import (
	"encoding/json"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.uber.org/zap"
)

type statusSessionHandler struct {
	conn    *minecraftConn
	inbound Inbound

	receivedRequest bool

	noOpSessionHandler
}

func (h *statusSessionHandler) activated() {
	cfg := h.conn.proxy.Config()
	if cfg.Status.ShowPingRequests || cfg.Debug {
		zap.S().Infof("%s with version %s", h.inbound, h.conn.protocol)
	}
}

func newStatusSessionHandler(conn *minecraftConn, inbound Inbound) sessionHandler {
	return &statusSessionHandler{conn: conn, inbound: inbound}
}

func (h *statusSessionHandler) handlePacket(p proto.Packet) {
	switch typed := p.(type) {
	case *packet.StatusRequest:
		h.handleStatusRequest()
	case *packet.StatusPing:
		h.handleStatusPing(typed)
	default:
		h.conn.close()
	}
}

var versionName = fmt.Sprintf("Gate %s", proto.SupportedVersionsString)

func (h *statusSessionHandler) newInitialPing() *ping.ServerPing {
	shownVersion := h.conn.Protocol()
	if !h.conn.Protocol().Supported() {
		shownVersion = proto.MaximumVersion.Protocol
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
		zap.L().Debug("Ping is nil, sent no response")
		return
	}
	if !h.inbound.Active() {
		return
	}

	response, err := json.Marshal(e.ping)
	if err != nil {
		_ = h.conn.close()
		zap.L().Error("Error marshaling ping response to json", zap.Error(err))
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
		zap.S().Debugf("Error writing StatusPing: %v", err)
	}
}

func (h *statusSessionHandler) handleUnknownPacket(p *proto.PacketContext) {
	// What even is going on? ;D
	h.conn.close()
}

func (h *statusSessionHandler) proxy() *Proxy {
	return h.conn.proxy
}

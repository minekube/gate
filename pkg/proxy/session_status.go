package proxy

import (
	"fmt"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.uber.org/zap"
	"strings"
)

type statusSessionHandler struct {
	conn    *minecraftConn
	inbound Inbound

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

func (h *statusSessionHandler) handleStatusRequest() {
	// TODO proxy ping event
	hover := new(strings.Builder)
	_ = (&legacy.Legacy{}).Marshal(hover, &component.Text{
		Content: "A Minecraft Proxy by Minekube",
		S:       component.Style{Color: color.Gold},
	})

	motd := new(strings.Builder)
	_ = (&codec.Json{}).Marshal(motd, &component.Text{
		Content: "A Gate Proxy ",
		S:       component.Style{Color: color.Aqua},
		Extra: []component.Component{
			&component.Text{
				Content: "(Alpha)\n",
				S:       component.Style{Color: color.Gray},
			},
			&component.Text{Content: "Visit ➞ "},
			&component.Text{
				Content: "github.com/minekube/gate",
				S:       component.Style{Color: color.White},
			},
		},
	})
	_ = h.conn.WritePacket(&packet.StatusResponse{
		Status: fmt.Sprintf(sampleStatus, h.conn.Protocol(), len(h.conn.proxy.connect.ids),
			hover.String(), motd.String()),
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

const sampleStatus = `{
    "version": {
        "name": "1.8.9",
        "protocol": %d
    },
    "players": {
        "max": 100,
        "online": %d,
        "sample": [
            {
                "name": "%s",
                "id": "00000000-0000-0000-0000-000000000000"
            }
        ]
    },	
    "description": %s
}`

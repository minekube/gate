package proxy

import (
	"fmt"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/internal/quotautil"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/proto/state"
	"go.minekube.com/gate/pkg/proxy/forge"
	"go.uber.org/zap"
	"net"
	"strconv"
	"strings"
)

type handshakeSessionHandler struct {
	conn *minecraftConn

	noOpSessionHandler
}

// newHandshakeSessionHandler returns a handler used for clients in the handshake state.
func newHandshakeSessionHandler(conn *minecraftConn) sessionHandler {
	return &handshakeSessionHandler{conn: conn}
}

func (h *handshakeSessionHandler) handlePacket(p proto.Packet) {
	switch typed := p.(type) {
	// TODO legacy pings
	case *packet.Handshake:
		h.handleHandshake(typed)
	default:
		// Unknown packet received. Better to close the connection.
		h.conn.close()
	}
}

func (h *handshakeSessionHandler) handleUnknownPacket(p *proto.PacketContext) {
	// Unknown packet received. Better to close the connection.
	h.conn.close()
}

func (h *handshakeSessionHandler) handleHandshake(handshake *packet.Handshake) {
	vHost := tcpAddr(net.JoinHostPort(handshake.ServerAddress, strconv.Itoa(int(handshake.Port))))
	inbound := newInitialInbound(h.conn, vHost)

	// The client sends the next wanted state in the Handshake packet.
	nextState := stateForProtocol(handshake.NextStatus)
	if nextState == nil {
		zap.S().Debugf("%s provided invalid protocol %d", inbound, handshake.NextStatus)
		h.conn.close()
		return
	}

	// Update connection to requested state and protocol sent in the packet.
	h.conn.SetState(nextState)
	h.conn.SetProtocol(proto.Protocol(handshake.ProtocolVersion))

	switch nextState {
	case state.Status:
		// Client wants to enter the Status state to get the server status.
		// Just update the session handler and wait for the StatusRequest packet.
		h.conn.setSessionHandler(newStatusSessionHandler(h.conn, inbound))
	case state.Login:
		// Client wants to join.
		h.handleLogin(handshake, inbound)
	}
}

func (h *handshakeSessionHandler) handleLogin(p *packet.Handshake, inbound Inbound) {
	// Check for supported client version.
	if !proto.Protocol(p.ProtocolVersion).Supported() {
		_ = h.conn.closeWith(packet.DisconnectWith(&component.Translation{
			Key: "multiplayer.disconnect.outdated_client",
		}))
		return
	}

	// Client IP-block rate limiter preventing too fast logins hitting the Mojang API
	if loginsQuota := h.loginsQuota(); loginsQuota != nil && loginsQuota.Blocked(inbound.RemoteAddr()) {
		_ = h.conn.closeWith(packet.DisconnectWith(&component.Text{
			Content: "You are logging in to fast, wait a little and retry.",
			S:       component.Style{Color: color.Red},
		}))
		return
	}
	h.conn.SetType(connTypeForHandshake(p))

	// If the proxy is configured for velocity's forwarding mode, we must deny connections from 1.12.2
	// and lower, otherwise IP information will never get forwarded.
	if h.conn.proxy.Config().Forwarding.Mode == config.VelocityForwardingMode &&
		p.ProtocolVersion < int(proto.Minecraft_1_13.Protocol) {
		_ = h.conn.closeWith(packet.DisconnectWith(&component.Text{
			Content: "This server is only compatible with versions 1.13 and above.",
		}))
		return
	}

	h.conn.proxy.Event().Fire(&ConnectionHandshakeEvent{inbound: inbound})
	h.conn.setSessionHandler(newLoginSessionHandler(h.conn, inbound))
}

func (h *handshakeSessionHandler) loginsQuota() *quotautil.Quota {
	return h.conn.proxy.Connect().loginsQuota
}

func stateForProtocol(status int) *state.Registry {
	switch proto.State(status) {
	case proto.StatusState:
		return state.Status
	case proto.LoginState:
		return state.Login
	}
	return nil
}

func connTypeForHandshake(h *packet.Handshake) connectionType {
	// Determine if we're using Forge (1.8 to 1.12, may not be the case in 1.13).
	if h.ProtocolVersion < int(proto.Minecraft_1_13.Protocol) &&
		strings.HasSuffix(h.ServerAddress, forge.HandshakeHostnameToken) {
		return LegacyForge
	} else if h.ProtocolVersion <= int(proto.Minecraft_1_7_6.Protocol) {
		// 1.7 Forge will not notify us during handshake. UNDETERMINED will listen for incoming
		// forge handshake attempts. Also sends a reset handshake packet on every transition.
		return undetermined17ConnectionType
	}
	// Note for future implementation: Forge 1.13+ identifies
	// itself using a slightly different hostname token.
	return vanillaConnectionType
}

type tcpAddr string

func (v tcpAddr) Network() string {
	return "tcp"
}

func (v tcpAddr) String() string {
	return string(v)
}

var _ net.Addr = (*tcpAddr)(nil)

type initialInbound struct {
	*minecraftConn
	virtualHost net.Addr
}

var _ Inbound = (*initialInbound)(nil)

func newInitialInbound(c *minecraftConn, virtualHost net.Addr) Inbound {
	return &initialInbound{
		minecraftConn: c,
		virtualHost:   virtualHost,
	}
}

func (i *initialInbound) VirtualHost() net.Addr {
	return i.virtualHost
}

func (i *initialInbound) Active() bool {
	return !i.Closed()
}

func (i *initialInbound) String() string {
	return fmt.Sprintf("[initial connection] %s -> %s", i.RemoteAddr(), i.virtualHost)
}

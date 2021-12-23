package proxy

import (
	"fmt"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/addrquota"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/netutil"
	"net"
	"strings"
)

type handshakeSessionHandler struct {
	conn *minecraftConn
	log  logr.Logger

	nopSessionHandler
}

// newHandshakeSessionHandler returns a handler used for clients in the handshake state.
func newHandshakeSessionHandler(conn *minecraftConn) sessionHandler {
	return &handshakeSessionHandler{conn: conn, log: conn.log.WithName("handshakeSession")}
}

func (h *handshakeSessionHandler) handlePacket(p *proto.PacketContext) {
	if !p.KnownPacket {
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.close()
		return
	}
	switch typed := p.Packet.(type) {
	// TODO legacy pings
	case *packet.Handshake:
		h.handleHandshake(typed)
	default:
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.close()
	}
}

func (h *handshakeSessionHandler) handleHandshake(handshake *packet.Handshake) {
	vHost := netutil.NewAddr("tcp", handshake.ServerAddress, uint16(handshake.Port))
	inbound := newInitialInbound(h.conn, vHost)

	// The client sends the next wanted state in the Handshake packet.
	nextState := stateForProtocol(handshake.NextStatus)
	if nextState == nil {
		h.log.V(1).Info("Client provided invalid next status state, closing connection",
			"nextStatus", handshake.NextStatus)
		_ = h.conn.close()
		return
	}

	// Update connection to requested state and protocol sent in the packet.
	h.conn.setState(nextState)
	h.conn.setProtocol(proto.Protocol(handshake.ProtocolVersion))

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
	if !version.Protocol(p.ProtocolVersion).Supported() {
		_ = h.conn.closeWith(packet.DisconnectWith(&component.Translation{
			Key: "multiplayer.disconnect.outdated_client",
		}))
		return
	}

	// Client IP-block rate limiter preventing too fast logins hitting the Mojang API
	if loginsQuota := h.loginsQuota(); loginsQuota != nil && loginsQuota.Blocked(netutil.Host(inbound.RemoteAddr())) {
		_ = h.conn.closeWith(packet.DisconnectWith(&component.Text{
			Content: "You are logging in too fast, please calm down and retry.",
			S:       component.Style{Color: color.Red},
		}))
		return
	}
	h.conn.setType(connTypeForHandshake(p))

	// If the proxy is configured for velocity's forwarding mode, we must deny connections from 1.12.2
	// and lower, otherwise IP information will never get forwarded.
	if h.proxy().Config().Forwarding.Mode == config.VelocityForwardingMode &&
		p.ProtocolVersion < int(version.Minecraft_1_13.Protocol) {
		_ = h.conn.closeWith(packet.DisconnectWith(&component.Text{
			Content: "This server is only compatible with versions 1.13 and above.",
		}))
		return
	}

	// TODO create LoginInboundConnection & Add support for sending and receiving login plugin messages from players and servers
	h.proxy().Event().Fire(&ConnectionHandshakeEvent{inbound: inbound})
	h.conn.setSessionHandler(newLoginSessionHandler(h.conn, inbound))
}

func (h *handshakeSessionHandler) proxy() *Proxy {
	return h.conn.proxy
}

func (h *handshakeSessionHandler) loginsQuota() *addrquota.Quota {
	return h.proxy().loginsQuota
}

func stateForProtocol(status int) *state.Registry {
	switch state.State(status) {
	case state.StatusState:
		return state.Status
	case state.LoginState:
		return state.Login
	}
	return nil
}

func connTypeForHandshake(h *packet.Handshake) connectionType {
	// Determine if we're using Forge (1.8 to 1.12, may not be the case in 1.13).
	if h.ProtocolVersion < int(version.Minecraft_1_13.Protocol) &&
		strings.HasSuffix(h.ServerAddress, forge.HandshakeHostnameToken) {
		return LegacyForge
	} else if h.ProtocolVersion <= int(version.Minecraft_1_7_6.Protocol) {
		// 1.7 Forge will not notify us during handshake. UNDETERMINED will listen for incoming
		// forge handshake attempts. Also sends a reset handshake packet on every transition.
		return undetermined17ConnectionType
	}
	// Note for future implementation: Forge 1.13+ identifies
	// itself using a slightly different hostname token.
	return vanillaConnectionType
}

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

func (i *initialInbound) Closed() <-chan struct{} {
	return i.minecraftConn.closed
}

func (i *initialInbound) VirtualHost() net.Addr {
	return i.virtualHost
}

func (i *initialInbound) Active() bool {
	return !i.minecraftConn.Closed()
}

func (i *initialInbound) String() string {
	return fmt.Sprintf("[initial connection] %s -> %s", i.RemoteAddr(), i.virtualHost)
}

//
//
//
//
//
//

// A no-operation session handler can be wrapped to
// implement the sessionHandler interface.
type nopSessionHandler struct{}

var _ sessionHandler = (*nopSessionHandler)(nil)

func (nopSessionHandler) handlePacket(*proto.PacketContext) {}
func (nopSessionHandler) disconnected()                     {}
func (nopSessionHandler) deactivated()                      {}
func (nopSessionHandler) activated()                        {}

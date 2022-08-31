package proxy

import (
	"fmt"
	"net"
	"strings"

	"github.com/go-logr/logr"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/forge"
	netmc "go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/addrquota"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/util/netutil"
)

type sessionHandlerDeps struct {
	proxy          *Proxy
	registrar      playerRegistrar
	players        playerProvider
	eventMgr       event.Manager
	configProvider configProvider
	authenticator  auth.Authenticator
	loginsQuota    *addrquota.Quota
}

func (d *sessionHandlerDeps) config() *config.Config {
	return d.configProvider.config()
}
func (d *sessionHandlerDeps) auth() auth.Authenticator {
	return d.authenticator
}

type handshakeSessionHandler struct {
	*sessionHandlerDeps

	conn netmc.MinecraftConn
	log  logr.Logger

	nopSessionHandler
}

// newHandshakeSessionHandler returns a handler used for clients in the handshake state.
func newHandshakeSessionHandler(
	conn netmc.MinecraftConn,
	deps *sessionHandlerDeps,
) netmc.SessionHandler {
	return &handshakeSessionHandler{
		sessionHandlerDeps: deps,
		conn:               conn,
		log:                logr.FromContextOrDiscard(conn.Context()).WithName("handshakeSession"),
	}
}

func (h *handshakeSessionHandler) HandlePacket(p *proto.PacketContext) {
	if !p.KnownPacket {
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.Close()
		return
	}
	switch typed := p.Packet.(type) {
	// TODO legacy pings
	case *packet.Handshake:
		h.handleHandshake(typed)
	default:
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.Close()
	}
}

func (h *handshakeSessionHandler) handleHandshake(handshake *packet.Handshake) {
	vHost := netutil.NewAddr(
		fmt.Sprintf("%s:%d", handshake.ServerAddress, handshake.Port),
		h.conn.LocalAddr().Network(),
	)
	inbound := newInitialInbound(h.conn, vHost)

	// The client sends the next wanted state in the Handshake packet.
	nextState := stateForProtocol(handshake.NextStatus)
	if nextState == nil {
		h.log.V(1).Info("Client provided invalid next status state, closing connection",
			"nextStatus", handshake.NextStatus)
		_ = h.conn.Close()
		return
	}

	// Update connection to requested state and protocol sent in the packet.
	h.conn.SetState(nextState)
	h.conn.SetProtocol(proto.Protocol(handshake.ProtocolVersion))

	switch nextState {
	case state.Status:
		// Client wants to enter the Status state to get the server status.
		// Just update the session handler and wait for the StatusRequest packet.
		h.conn.SetSessionHandler(newStatusSessionHandler(h.conn, inbound, h.sessionHandlerDeps))
	case state.Login:
		// Client wants to join.
		h.handleLogin(handshake, inbound)
	}
}

func (h *handshakeSessionHandler) handleLogin(p *packet.Handshake, inbound *initialInbound) {
	// Check for supported client version.
	if !version.Protocol(p.ProtocolVersion).Supported() {
		_ = inbound.disconnect(&component.Translation{
			Key: "multiplayer.disconnect.outdated_client",
		})
		return
	}

	// Client IP-block rate limiter preventing too fast logins hitting the Mojang API
	if h.loginsQuota != nil && h.loginsQuota.Blocked(netutil.Host(inbound.RemoteAddr())) {
		_ = netmc.CloseWith(h.conn, packet.DisconnectWith(&component.Text{
			Content: "You are logging in too fast, please calm down and retry.",
			S:       component.Style{Color: color.Red},
		}))
		return
	}

	h.conn.SetType(connTypeForHandshake(p))

	// If the proxy is configured for velocity's forwarding mode, we must deny connections from 1.12.2
	// and lower, otherwise IP information will never get forwarded.
	if h.config().Forwarding.Mode == config.VelocityForwardingMode &&
		p.ProtocolVersion < int(version.Minecraft_1_13.Protocol) {
		_ = netmc.CloseWith(h.conn, packet.DisconnectWith(&component.Text{
			Content: "This server is only compatible with versions 1.13 and above.",
		}))
		return
	}

	lic := newLoginInboundConn(inbound)
	h.eventMgr.Fire(&ConnectionHandshakeEvent{inbound: lic})
	h.conn.SetSessionHandler(newInitialLoginSessionHandler(h.conn, lic, h.sessionHandlerDeps))
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

func connTypeForHandshake(h *packet.Handshake) phase.ConnectionType {
	// Determine if we're using Forge (1.8 to 1.12, may not be the case in 1.13).
	if h.ProtocolVersion < int(version.Minecraft_1_13.Protocol) &&
		strings.HasSuffix(h.ServerAddress, forge.HandshakeHostnameToken) {
		return phase.LegacyForge
	} else if h.ProtocolVersion <= int(version.Minecraft_1_7_6.Protocol) {
		// 1.7 Forge will not notify us during handshake. UNDETERMINED will listen for incoming
		// forge handshake attempts. Also sends a reset handshake packet on every transition.
		return phase.Undetermined17
	}
	// Note for future implementation: Forge 1.13+ identifies
	// itself using a slightly different hostname token.
	return phase.Vanilla
}

type initialInbound struct {
	netmc.MinecraftConn
	virtualHost net.Addr
}

var _ Inbound = (*initialInbound)(nil)

func newInitialInbound(c netmc.MinecraftConn, virtualHost net.Addr) *initialInbound {
	return &initialInbound{
		MinecraftConn: c,
		virtualHost:   virtualHost,
	}
}

func (i *initialInbound) VirtualHost() net.Addr {
	return i.virtualHost
}

func (i *initialInbound) Active() bool {
	return !netmc.Closed(i.MinecraftConn)
}

func (i *initialInbound) String() string {
	return fmt.Sprintf("[initial connection] %s -> %s", i.RemoteAddr(), i.virtualHost)
}

func (i *initialInbound) disconnect(reason component.Component) error {
	// TODO add cfg option to log player connections to log "player disconnected"
	return netmc.CloseWith(i.MinecraftConn, packet.DisconnectWithProtocol(reason, i.Protocol()))
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

var _ netmc.SessionHandler = (*nopSessionHandler)(nil)

func (nopSessionHandler) HandlePacket(*proto.PacketContext) {}
func (nopSessionHandler) Disconnected()                     {}
func (nopSessionHandler) Deactivated()                      {}
func (nopSessionHandler) Activated()                        {}

package proxy

import (
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/forge/modernforge"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/lite"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/addrquota"
	"go.minekube.com/gate/pkg/util/netutil"
)

type sessionHandlerDeps struct {
	proxy          *Proxy
	registrar      playerRegistrar
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
	if !p.KnownPacket() {
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.Close()
		return
	}
	switch typed := p.Packet.(type) {
	// TODO legacy pings
	case *packet.Handshake:
		h.handleHandshake(typed, p)
	default:
		// Unknown packet received.
		// Better to close the connection.
		_ = h.conn.Close()
	}
}

func (h *handshakeSessionHandler) handleHandshake(handshake *packet.Handshake, pc *proto.PacketContext) {
	// The client sends the next wanted state in the Handshake packet.
	nextState := stateForProtocol(handshake.NextStatus)
	if nextState == nil {
		h.log.V(1).Info("client provided invalid next status state, closing connection",
			"nextStatus", handshake.NextStatus)
		_ = h.conn.Close()
		return
	}

	// Update connection to requested state and protocol sent in the packet.
	h.conn.SetProtocol(proto.Protocol(handshake.ProtocolVersion))

	// Lite mode ping resolver
	var resolvePingResponse pingResolveFunc
	if h.config().Lite.Enabled {
		h.conn.SetState(nextState)
		dialTimeout := time.Duration(h.config().ConnectionTimeout)
		if nextState == state.Login {
			// Lite mode enabled, pipe the connection.
			lite.Forward(dialTimeout, h.config().Lite.Routes, h.log, h.conn, handshake, pc)
			return
		}
		// Resolve ping response for lite mode.
		resolvePingResponse = func(log logr.Logger, statusRequestCtx *proto.PacketContext) (logr.Logger, *packet.StatusResponse, error) {
			return lite.ResolveStatusResponse(dialTimeout, h.config().Lite.Routes, log, h.conn, handshake, pc, statusRequestCtx)
		}
	}

	vHost := netutil.NewAddr(
		fmt.Sprintf("%s:%d", handshake.ServerAddress, handshake.Port),
		h.conn.LocalAddr().Network(),
	)
	handshakeIntent := handshake.Intent()
	inbound := newInitialInbound(h.conn, vHost, handshakeIntent)

	if handshakeIntent == packet.TransferHandshakeIntent && !h.config().AcceptTransfers {
		_ = inbound.disconnect(&component.Translation{Key: "multiplayer.disconnect.transfers_disabled"})
		return
	}

	switch nextState {
	case state.Status:
		// Client wants to enter the Status state to get the server status.
		// Just update the session handler and wait for the StatusRequest packet.
		handler := newStatusSessionHandler(h.conn, inbound, h.sessionHandlerDeps, resolvePingResponse)
		h.conn.SetActiveSessionHandler(state.Status, handler)
	case state.Login:
		// Client wants to join.
		h.handleLogin(handshake, inbound)
	}
}

func (h *handshakeSessionHandler) handleLogin(p *packet.Handshake, inbound *initialInbound) {
	// Check for supported client version.
	if !version.Protocol(p.ProtocolVersion).Supported() {
		_ = inbound.disconnect(&component.Translation{
			Key:  "multiplayer.disconnect.outdated_client",
			With: []component.Component{&component.Text{Content: version.SupportedVersionsString}},
		})
		return
	}

	// Client IP-block rate limiter preventing too fast logins hitting the Mojang API
	if h.loginsQuota != nil && h.loginsQuota.Blocked(netutil.Host(inbound.RemoteAddr())) {
		_ = netmc.CloseWith(h.conn, packet.NewDisconnect(&component.Text{
			Content: "You are logging in too fast, please calm down and retry.",
			S:       component.Style{Color: color.Red},
		}, proto.Protocol(p.ProtocolVersion), h.conn.State().State))
		return
	}

	h.conn.SetType(handshakeConnectionType(p))

	// If the proxy is configured for velocity's forwarding mode, we must deny connections from 1.12.2
	// and lower, otherwise IP information will never get forwarded.
	if h.config().Forwarding.Mode == config.VelocityForwardingMode &&
		p.ProtocolVersion < int(version.Minecraft_1_13.Protocol) {
		_ = netmc.CloseWith(h.conn, packet.NewDisconnect(&component.Text{
			Content: "This server is only compatible with versions 1.13 and above.",
		}, proto.Protocol(p.ProtocolVersion), h.conn.State().State))
		return
	}

	lic := newLoginInboundConn(inbound)
	h.eventMgr.Fire(&ConnectionHandshakeEvent{inbound: lic, intent: p.Intent()})
	handler := newInitialLoginSessionHandler(h.conn, lic, h.sessionHandlerDeps)
	h.conn.SetActiveSessionHandler(state.Login, handler)
}

func stateForProtocol(status int) *state.Registry {

	switch states.State(status) {
	case states.StatusState:
		return state.Status
	case states.LoginState, states.State(packet.TransferHandshakeIntent):
		return state.Login
	}
	return nil
}

func handshakeConnectionType(h *packet.Handshake) phase.ConnectionType {
	if strings.Contains(h.ServerAddress, modernforge.Token) &&
		h.ProtocolVersion >= int(version.Minecraft_1_20_2.Protocol) {
		return phase.ModernForge
	}
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
	handshakeIntent packet.HandshakeIntent
}


var _ Inbound = (*initialInbound)(nil)

func newInitialInbound(c netmc.MinecraftConn, virtualHost net.Addr, handshakeIntent packet.HandshakeIntent) *initialInbound {
	return &initialInbound{
		MinecraftConn: c,
		virtualHost:   virtualHost,
		handshakeIntent: handshakeIntent,
	}
}

func (i *initialInbound) VirtualHost() net.Addr {
	return i.virtualHost
}

func (i *initialInbound) HandshakeIntent() packet.HandshakeIntent {
	return i.handshakeIntent
}

func (i *initialInbound) Active() bool {
	return !netmc.Closed(i.MinecraftConn)
}

func (i *initialInbound) String() string {
	return fmt.Sprintf("[initial connection] %s -> %s", i.RemoteAddr(), i.virtualHost)
}

func (i *initialInbound) disconnect(reason component.Component) error {
	// TODO add cfg option to log player connections to log "player disconnected"
	return netmc.CloseWith(i.MinecraftConn, packet.NewDisconnect(reason, i.Protocol(), i.State().State))
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

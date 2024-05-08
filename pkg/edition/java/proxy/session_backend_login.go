package proxy

import (
	"errors"
	"go.minekube.com/gate/pkg/edition/java/internal/velocity"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"reflect"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.uber.org/atomic"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/netutil"
)

type backendLoginSessionHandler struct {
	*sessionHandlerDeps

	serverConn    *serverConnection
	requestCtx    *connRequestCxt
	listenDoneCtx chan struct{}
	log           logr.Logger

	informationForwarded atomic.Bool
}

var _ netmc.SessionHandler = (*backendLoginSessionHandler)(nil)

func newBackendLoginSessionHandler(
	serverConn *serverConnection,
	requestCtx *connRequestCxt,
	sessionHandlerDeps *sessionHandlerDeps,
) netmc.SessionHandler {
	return &backendLoginSessionHandler{
		serverConn:         serverConn,
		requestCtx:         requestCtx,
		log:                serverConn.log.WithName("backendLoginSession"),
		sessionHandlerDeps: sessionHandlerDeps,
	}
}

func (b *backendLoginSessionHandler) Activated() {
	b.listenDoneCtx = make(chan struct{})
	go func() {
		select {
		case <-b.listenDoneCtx:
		case <-b.requestCtx.Done():
			// We must check again since request context
			// may be canceled before Deactivated() was run.
			select {
			case <-b.listenDoneCtx:
				return
			default:
				b.requestCtx.result(nil, errors.New(
					"context deadline exceeded while logging into backend server"))
				b.serverConn.disconnect()
			}
		}
	}()
}

func (b *backendLoginSessionHandler) Deactivated() {
	if b.listenDoneCtx != nil {
		close(b.listenDoneCtx)
	}
}

func (b *backendLoginSessionHandler) HandlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket() {
		return // ignore unknown
	}

	switch p := pc.Packet.(type) {
	case *packet.LoginPluginMessage:
		b.handleLoginPluginMessage(p)
	case *packet.Disconnect:
		b.handleDisconnect(p)
	case *packet.EncryptionRequest:
		b.handleEncryptionRequest()
	case *packet.SetCompression:
		b.handleSetCompression(p)
	case *packet.ServerLoginSuccess:
		b.handleServerLoginSuccess()
	default:
		b.log.V(1).Info("Received unexpected packet from backend server while logging in",
			"packetType", reflect.TypeOf(p))
	}
}

// ErrServerOnlineMode indicates error in a ConnectionRequest when the backend server is in online mode.
var ErrServerOnlineMode = errors.New("backend server is online mode, but should be offline")

func (b *backendLoginSessionHandler) handleEncryptionRequest() {
	// If we get an encryption request we know that the server is online mode or does not support tunneling!
	// Server should be offline mode.
	b.requestCtx.result(nil, ErrServerOnlineMode)
}

func (b *backendLoginSessionHandler) handleLoginPluginMessage(p *packet.LoginPluginMessage) {
	mc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}
	cfg := b.config()
	if cfg.Forwarding.Mode == config.VelocityForwardingMode && p.Channel == velocity.IpForwardingChannel {

		requestedForwardingVersion := velocity.DefaultForwardingVersion
		// Check version
		if len(p.Data) == 1 {
			requestedForwardingVersion = int(p.Data[0])
		}

		forwardingData, err := velocity.CreateForwardingData(
			[]byte(cfg.Forwarding.VelocitySecret),
			netutil.Host(b.serverConn.Player().RemoteAddr()),
			b.serverConn.player, requestedForwardingVersion,
		)
		if err != nil {
			b.log.Error(err, "error creating velocity forwarding data")
			b.serverConn.disconnect()
			return
		}
		if mc.WritePacket(&packet.LoginPluginResponse{
			ID:      p.ID,
			Success: true,
			Data:    forwardingData,
		}) != nil {
			return
		}
		b.informationForwarded.Store(true)
	} else {
		// Don't understand, fire event if we have subscribers
		if !b.eventMgr.HasSubscriber(&ServerLoginPluginMessageEvent{}) {
			_ = mc.WritePacket(&packet.LoginPluginResponse{
				ID:      p.ID,
				Success: false,
			})
			return
		}

		identifier, err := message.ChannelIdentifierFrom(p.Channel)
		if err != nil {
			b.log.V(1).Error(err, "could not parse channel from LoginPluginResponse")
			return
		}
		e := &ServerLoginPluginMessageEvent{
			id:         identifier,
			contents:   p.Data,
			sequenceID: p.ID,
		}
		b.eventMgr.Fire(e)
		if e.Result().Allowed() {
			_ = mc.WritePacket(&packet.LoginPluginResponse{
				ID:      p.ID,
				Success: true,
				Data:    e.Result().Response,
			})
			return
		}
		_ = mc.WritePacket(&packet.LoginPluginResponse{
			ID:      p.ID,
			Success: false,
		})
	}
}

func (b *backendLoginSessionHandler) handleDisconnect(p *packet.Disconnect) {
	result := disconnectResultForPacket(b.log.V(1), p, b.serverConn.player.Protocol(), b.serverConn.server, true)
	b.requestCtx.result(result, nil)
	b.serverConn.disconnect()
}

func (b *backendLoginSessionHandler) handleSetCompression(packet *packet.SetCompression) {
	conn, ok := b.serverConn.ensureConnected()
	if ok {
		if err := conn.SetCompressionThreshold(packet.Threshold); err != nil {
			b.requestCtx.result(nil, err)
			b.serverConn.disconnect()
		}
	}
}

var velocityIpForwardingFailure = &component.Text{
	Content: "Your server did not send a forwarding request to the proxy. Is velocity forwarding set up correctly?",
}

func (b *backendLoginSessionHandler) handleServerLoginSuccess() {
	if b.config().Forwarding.Mode == config.VelocityForwardingMode && !b.informationForwarded.Load() {
		b.requestCtx.result(disconnectResult(velocityIpForwardingFailure, b.serverConn.server, true), nil)
		b.serverConn.disconnect()
		return
	}

	// The player has been logged on to the backend server, but we're not done yet. There could be
	// other problems that could arise before we get a JoinGame packet from the server.

	// Move into the PLAY phase.
	serverMc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}

	if serverMc.Protocol().Lower(version.Minecraft_1_20_2) {
		serverMc.SetActiveSessionHandler(state.Play,
			newBackendTransitionSessionHandler(b.serverConn, b.requestCtx, b.proxy))
	} else {
		fail := func(err error) {
			b.log.V(1).Error(err, "error transitioning to backend config state")
			b.requestCtx.result(nil, err)
			b.serverConn.disconnect()
		}
		err := serverMc.WritePacket(&packet.LoginAcknowledged{})
		if err != nil {
			fail(err)
			return
		}
		sh, err := newBackendConfigSessionHandler(b.serverConn, b.requestCtx)
		if err != nil {
			fail(err)
			return
		}
		serverMc.SetActiveSessionHandler(state.Config, sh)
		player := b.serverConn.player
		if pkt := player.ClientSettingsPacket(); pkt != nil {
			err = serverMc.WritePacket(pkt)
			if err != nil {
				fail(err)
				return
			}
		}

		ash := player.ActiveSessionHandler()
		csh, ok := ash.(*clientPlaySessionHandler)
		if ok {
			serverMc.SetAutoReading(false)
			csh.doSwitch().ThenAccept(func(any) {
				serverMc.SetAutoReading(true)
			})
		}
	}
}

func (b *backendLoginSessionHandler) Disconnected() {
	if b.config().Forwarding.Mode == config.LegacyForwardingMode || b.config().Forwarding.Mode == config.BungeeGuardFowardingMode {
		b.requestCtx.result(nil, errs.NewSilentErr(`The connection to the remote server was unexpectedly closed.
This is usually because the remote server does not have BungeeCord IP forwarding correctly enabled.`))
	} else {
		b.requestCtx.result(nil, errs.NewSilentErr("The connection to the remote server was unexpectedly closed."))
	}
}

func disconnectResultForPacket(
	errLog logr.Logger,
	p *packet.Disconnect,
	protocol proto.Protocol,
	server RegisteredServer,
	safe bool,
) *connectionResult {
	var reason *chat.ComponentHolder
	if p != nil && p.Reason != nil {
		reason = p.Reason
	}
	return disconnectResult(reason.AsComponentOrNil(), server, safe)
}
func disconnectResult(reason component.Component, server RegisteredServer, safe bool) *connectionResult {
	return &connectionResult{
		status:        ServerDisconnectedConnectionStatus,
		reason:        reason,
		safe:          safe,
		attemptedConn: server,
	}
}

func (b *backendLoginSessionHandler) config() *config.Config {
	return b.configProvider.config()
}

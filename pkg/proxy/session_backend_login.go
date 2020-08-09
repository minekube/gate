package proxy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/proto/state"
	protoutil "go.minekube.com/gate/pkg/proto/util"
	"go.minekube.com/gate/pkg/util"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/profile"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"reflect"
	"strings"
)

type backendLoginSessionHandler struct {
	serverConn    *serverConnection
	requestCtx    *connRequestCxt
	listenDoneCtx chan struct{}

	informationForwarded atomic.Bool

	noOpSessionHandler
}

var _ sessionHandler = (*backendLoginSessionHandler)(nil)

func newBackendLoginSessionHandler(serverConn *serverConnection, requestCtx *connRequestCxt) sessionHandler {
	return &backendLoginSessionHandler{serverConn: serverConn, requestCtx: requestCtx}
}

func (b *backendLoginSessionHandler) activated() {
	b.listenDoneCtx = make(chan struct{})
	go func() {
		select {
		case <-b.listenDoneCtx:
		case <-b.requestCtx.Done():
			// We must check again since request context
			// may be canceled before deactivated() was run.
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

func (b *backendLoginSessionHandler) deactivated() {
	if b.listenDoneCtx != nil {
		close(b.listenDoneCtx)
	}
}

func (b *backendLoginSessionHandler) handlePacket(p proto.Packet) {
	switch t := p.(type) {
	case *packet.LoginPluginMessage:
		b.handleLoginPluginMessage(t)
	case *packet.Disconnect:
		b.handleDisconnect(t)
	case *packet.EncryptionRequest:
		b.handleEncryptionRequest()
	case *packet.SetCompression:
		b.handleSetCompression(t)
	case *packet.ServerLoginSuccess:
		b.handleServerLoginSuccess()
	default:
		zap.L().Warn("Received unhandled packet from backend server while logging in",
			zap.Stringer("type", reflect.TypeOf(p)))
	}
}

// An error in a ConnectionRequest when the backend server is in online mode.
var ErrServerOnlineMode = errors.New("backend server is online mode, but should be offline")

func (b *backendLoginSessionHandler) handleEncryptionRequest() {
	// If we get an encryption request we know that the server is online mode!
	// Server should be offline mode.
	b.requestCtx.result(nil, ErrServerOnlineMode)
}

const (
	velocityIpForwardingChannel = "velocity:player_info"
	velocityForwardingVersion   = 1
)

func (b *backendLoginSessionHandler) handleLoginPluginMessage(p *packet.LoginPluginMessage) {
	mc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}
	cfg := b.config()
	if cfg.Forwarding.Mode == config.VelocityForwardingMode &&
		strings.EqualFold(p.Channel, velocityIpForwardingChannel) {
		forwardingData, err := createVelocityForwardingData([]byte(cfg.Forwarding.VelocitySecret),
			b.serverConn.Player().RemoteAddr().String(),
			b.serverConn.player.profile)
		if err != nil {
			zap.L().Error("Error creating velocity forwarding data", zap.Error(err))
			return
		}
		if mc.WritePacket(&packet.LoginPluginResponse{
			Id:      p.Id,
			Success: true,
			Data:    forwardingData,
		}) != nil {
			return
		}
		b.informationForwarded.Store(true)
	} else {
		// Don't understand
		_ = mc.WritePacket(&packet.LoginPluginResponse{
			Id:      p.Id,
			Success: false,
		})
	}
}

func createVelocityForwardingData(hmacSecret []byte, address string, profile *profile.GameProfile) ([]byte, error) {
	forwarded := bytes.NewBuffer(make([]byte, 2048))
	err := protoutil.WriteVarInt(forwarded, velocityForwardingVersion)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteString(forwarded, address)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteUuid(forwarded, profile.Id)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteString(forwarded, profile.Name)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteProperties(forwarded, profile.Properties)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, hmacSecret)
	_, err = mac.Write(forwarded.Bytes())
	if err != nil {
		return nil, err
	}

	// final
	data := bytes.NewBuffer(make([]byte, mac.Size()+forwarded.Len()))
	_, err = data.Write(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	_, err = data.Write(forwarded.Bytes())
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func (b *backendLoginSessionHandler) handleDisconnect(p *packet.Disconnect) {
	result := disconnectResultForPacket(p, b.serverConn.player.Protocol(), b.serverConn.server, true)
	b.requestCtx.result(result, nil)
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
	serverMc.setState(state.Play)

	// Switch to the transition handler.
	serverMc.setSessionHandler(newBackendTransitionSessionHandler(b.serverConn, b.requestCtx))
}

func (b *backendLoginSessionHandler) disconnected() {
	if b.config().Forwarding.Mode == config.LegacyForwardingMode {
		b.requestCtx.result(nil, errs.NewSilentErr(`The connection to the remote server was unexpectedly closed.
This is usually because the remote server does not have BungeeCord IP forwarding correctly enabled.`))
		// TODO add link to player info forwarding instructions docs
	} else {
		b.requestCtx.result(nil, errs.NewSilentErr("The connection to the remote server was unexpectedly closed."))
	}
}

func disconnectResultForPacket(
	p *packet.Disconnect,
	protocol proto.Protocol,
	server RegisteredServer,
	safe bool,
) *connectionResult {
	var reason string
	if p != nil && p.Reason != nil {
		reason = *p.Reason
	}
	r, _ := util.JsonCodec(protocol).Unmarshal([]byte(reason))
	return disconnectResult(r, server, safe)
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
	return b.serverConn.player.proxy.config
}

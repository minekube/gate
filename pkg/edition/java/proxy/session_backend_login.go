package proxy

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"reflect"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/atomic"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
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

	nopSessionHandler
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
	if !pc.KnownPacket {
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

const (
	velocityIpForwardingChannel        = "velocity:player_info"
	velocityDefaultForwardingVersion   = 1
	velocityWithKeyForwardingVersion   = 2
	velocityWithKeyV2ForwardingVersion = 3
	velocityForwardingMaxVersion       = velocityWithKeyV2ForwardingVersion
)

func (b *backendLoginSessionHandler) handleLoginPluginMessage(p *packet.LoginPluginMessage) {
	mc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}
	cfg := b.config()
	if cfg.Forwarding.Mode == config.VelocityForwardingMode && p.Channel == velocityIpForwardingChannel {

		requestedForwardingVersion := velocityDefaultForwardingVersion
		// Check version
		if len(p.Data) == 1 {
			requestedForwardingVersion = int(p.Data[0])
		}

		forwardingData, err := createVelocityForwardingData(
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

// find velocity forwarding version
func findForwardingVersion(requested int, player *connectedPlayer) int {
	// Ensure we are in range
	requested = min(requested, velocityForwardingMaxVersion)
	if requested > velocityDefaultForwardingVersion {
		if revision := player.IdentifiedKey().KeyRevision(); revision != nil {
			switch revision {
			case keyrevision.GenericV1:
				return velocityWithKeyForwardingVersion
			// Since V2 is not backwards compatible we have to throw the key if v2 and requested is v1
			case keyrevision.LinkedV2:
				if requested >= velocityWithKeyV2ForwardingVersion {
					return velocityWithKeyV2ForwardingVersion
				}
				return velocityDefaultForwardingVersion
			}
		}
	}
	return velocityDefaultForwardingVersion
}

func createVelocityForwardingData(
	hmacSecret []byte, address string,
	player *connectedPlayer, requestedVersion int,
) ([]byte, error) {
	forwarded := bytes.NewBuffer(make([]byte, 0, 2048))

	actualVersion := findForwardingVersion(requestedVersion, player)

	err := protoutil.WriteVarInt(forwarded, actualVersion)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteString(forwarded, address)
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteUUID(forwarded, player.ID())
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteString(forwarded, player.Username())
	if err != nil {
		return nil, err
	}
	err = protoutil.WriteProperties(forwarded, player.GameProfile().Properties)
	if err != nil {
		return nil, err
	}

	// This serves as additional redundancy. The key normally is stored in the
	// login start to the server, but some setups require this.
	if actualVersion >= velocityWithKeyForwardingVersion {
		playerKey := player.IdentifiedKey()
		if playerKey == nil {
			return nil, errors.New("player auth key missing")
		}
		err = crypto.WritePlayerKey(forwarded, playerKey)
		if err != nil {
			return nil, err
		}

		// Provide the signer UUID since the UUID may differ from the
		// assigned UUID. Doing that breaks the signatures anyway but the server
		// should be able to verify the key independently.
		if actualVersion >= velocityWithKeyV2ForwardingVersion {
			if playerKey.SignatureHolder() != uuid.Nil {
				_ = protoutil.WriteBool(forwarded, true)
				_ = protoutil.WriteUUID(forwarded, playerKey.SignatureHolder())
			} else {
				// Should only not be provided if the player was connected
				// as offline-mode and the signer UUID was not backfilled
				_ = protoutil.WriteBool(forwarded, false)
			}
		}
	}

	mac := hmac.New(sha256.New, hmacSecret)
	_, err = mac.Write(forwarded.Bytes())
	if err != nil {
		return nil, err
	}

	// final
	data := bytes.NewBuffer(make([]byte, 0, mac.Size()+forwarded.Len()))
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
	serverMc.SetState(state.Play)

	// Switch to the transition handler.
	serverMc.SetSessionHandler(newBackendTransitionSessionHandler(b.serverConn, b.requestCtx, b.eventMgr, b.proxy))
}

func (b *backendLoginSessionHandler) Disconnected() {
	if b.config().Forwarding.Mode == config.LegacyForwardingMode {
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
	var reason string
	if p != nil && p.Reason != nil {
		reason = *p.Reason
	}
	r, err := protoutil.JsonCodec(protocol).Unmarshal([]byte(reason))
	if errLog.Enabled() && err != nil {
		errLog.Error(err, "Error unmarshal disconnect reason from server",
			"safe", safe, "protocol", protocol,
			"reason", reason, "server", server.ServerInfo().Name())
	}
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
	return b.configProvider.config()
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

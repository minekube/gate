package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"time"

	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/util/netutil"
)

type initialLoginSessionHandler struct {
	conn    *minecraftConn
	inbound *loginInboundConn
	log     logr.Logger

	nopSessionHandler

	currentState loginState
	login        *packet.ServerLogin
	verify       []byte
}

type loginState string

const (
	loginPacketExpectedLoginState        loginState = "loginPacketExpected"
	loginPacketReceivedLoginState        loginState = "loginPacketReceived"
	encryptionRequestSentLoginState      loginState = "encryptionRequestSent"
	encryptionResponseReceivedLoginState loginState = "encryptionResponseReceived"
)

func newInitialLoginSessionHandler(conn *minecraftConn, inbound *loginInboundConn) sessionHandler {
	return &initialLoginSessionHandler{
		conn:         conn,
		inbound:      inbound,
		log:          conn.log.WithName("loginSession"),
		currentState: loginPacketExpectedLoginState,
	}
}

var invalidPlayerName = &component.Text{
	Content: "Your username has an invalid format.",
	S:       component.Style{Color: color.Red},
}

func (l *initialLoginSessionHandler) handlePacket(p *proto.PacketContext) {
	if !p.KnownPacket {
		// unknown packet, close connection
		_ = l.conn.closeKnown(true)
		return
	}
	switch t := p.Packet.(type) {
	// TODO fire ServerLoginPluginMessageEvent
	case *packet.ServerLogin:
		l.handleServerLogin(t)
	case *packet.LoginPluginResponse:
		_ = l.inbound.handleLoginPluginResponse(t)
	case *packet.EncryptionResponse:
		l.handleEncryptionResponse(t)
	default:
		// got unexpected packet, simple close
		_ = l.conn.close()
	}
}

var playerNameRegex = regexp.MustCompile(`^[A-Za-z0-9_]{2,16}$`)

// GameProfileProvider provides the GameProfile for a player connection.
type GameProfileProvider interface {
	GameProfile() (profile *profile.GameProfile)
}

func (l *initialLoginSessionHandler) handleServerLogin(login *packet.ServerLogin) {
	if !l.assertState(loginPacketExpectedLoginState) {
		return
	}
	l.currentState = loginPacketReceivedLoginState

	playerKey := login.PlayerKey
	if playerKey != nil {
		if playerKey.Expired() {
			l.log.V(1).Info("expired player public key")
			_ = l.inbound.disconnect(&component.Translation{
				Key: "multiplayer.disconnect.invalid_public_key_signature",
			})
			return
		}

		// todo SignatureValid
		//if !playerKey.SignatureValid() {
		//	l.log.V(1).Info("invalid player public key signature")
		//	_ = l.inbound.disconnect(&component.Translation{
		//		Key: "multiplayer.disconnect.invalid_public_key",
		//	})
		//	return
		//}
	} else if l.conn.Protocol().GreaterEqual(version.Minecraft_1_19) &&
		l.proxy().config.ForceKeyAuthentication {
		_ = l.inbound.disconnect(&component.Translation{
			Key: "multiplayer.disconnect.missing_public_key",
		})
		return
	}
	l.inbound.playerKey = playerKey
	l.login = login

	// Validate username format
	if !playerNameRegex.MatchString(login.Username) {
		_ = l.inbound.disconnect(invalidPlayerName)
		return
	}

	e := newPreLoginEvent(l.inbound, l.login.Username)
	l.event().Fire(e)

	if l.conn.Closed() {
		return // Player was disconnected
	}

	if e.Result() == DeniedPreLogin {
		_ = l.inbound.disconnect(e.Reason())
		return
	}

	_ = l.inbound.loginEventFired(func() error {
		if l.conn.Closed() {
			return nil // Player was disconnected
		}

		if e.Result() != ForceOfflineModePreLogin &&
			(e.Result() == ForceOnlineModePreLogin || l.config().OnlineMode) {

			if p, ok := l.conn.c.(GameProfileProvider); ok {
				sh := newAuthSessionHandler(l.inbound, p.GameProfile(), false)
				l.conn.setSessionHandler0(sh)
				return nil
			}

			// Online mode login, send encryption request
			request := l.generateEncryptionRequest()
			l.verify = make([]byte, len(request.VerifyToken))
			copy(l.verify, request.VerifyToken)
			err := l.conn.WritePacket(request)
			if err != nil {
				return err
			}
			l.currentState = encryptionRequestSentLoginState
			// Wait for EncryptionResponse packet
			return nil
		}

		// Offline mode login
		sh := newAuthSessionHandler(l.inbound, profile.NewOffline(l.login.Username), false)
		l.conn.setSessionHandler0(sh)
		return nil
	})
}

func (l *initialLoginSessionHandler) generateEncryptionRequest() *packet.EncryptionRequest {
	verify := make([]byte, 4)
	_, _ = rand.Read(verify)
	return &packet.EncryptionRequest{
		PublicKey:   l.auth().PublicKey(),
		VerifyToken: verify,
	}
}

var unableAuthWithMojang = &component.Text{
	Content: "Unable to authenticate you with Mojang.\nPlease try again!",
	S:       component.Style{Color: color.Red},
}

func (l *initialLoginSessionHandler) handleEncryptionResponse(resp *packet.EncryptionResponse) {
	if !l.assertState(encryptionRequestSentLoginState) {
		return
	}
	l.currentState = encryptionResponseReceivedLoginState

	if l.login == nil {
		l.conn.log.V(1).Info("no ServerLogin packet received yet, disconnecting")
		_ = l.conn.closeKnown(true)
		return
	}
	if len(l.verify) == 0 {
		l.conn.log.V(1).Info("no EncryptionRequest packet sent yet, disconnecting")
		_ = l.conn.closeKnown(true)
		return
	}

	if playerKey := l.inbound.IdentifiedKey(); playerKey != nil {
		if resp.Salt == nil {
			l.conn.log.V(1).Info("encryption response did not contain salt")
			_ = l.conn.closeKnown(true)
			return
		}
		salt := new(bytes.Buffer)
		_ = util.WriteInt64(salt, *resp.Salt)
		valid := playerKey.VerifyDataSignature(resp.VerifyToken, l.verify, salt.Bytes())
		if !valid {
			l.conn.log.Info("invalid client public signature")
			_ = l.conn.closeKnown(true)
			return
		}
	} else {
		valid, err := l.auth().Verify(resp.VerifyToken, l.verify)
		if err != nil {
			// Simply close the connection without much overhead.
			_ = l.conn.closeKnown(true)
			return
		}
		if !valid {
			l.conn.log.Info("invalid verification token")
			_ = l.conn.closeKnown(true)
			return
		}
	}

	authn := l.auth()
	decryptedSharedSecret, err := authn.DecryptSharedSecret(resp.SharedSecret)
	if err != nil {
		// Simple close the connection without much overhead.
		_ = l.conn.close()
		return
	}

	// Enable encryption.
	// Once the client sends EncryptionResponse, encryption is enabled.
	if err = l.conn.enableEncryption(decryptedSharedSecret); err != nil {
		l.log.Error(err, "error enabling encryption for connecting player")
		_ = l.conn.closeWith(packet.DisconnectWith(internalServerConnectionError))
		return
	}

	var optionalUserIP string
	if l.config().ShouldPreventClientProxyConnections {
		optionalUserIP = netutil.Host(l.conn.RemoteAddr())
	}

	serverID, err := authn.GenerateServerID(decryptedSharedSecret)
	if err != nil {
		// Simple close the connection without much overhead.
		_ = l.inbound.disconnect(unableAuthWithMojang)
		return
	}

	log := l.log.WithName("authn")
	ctx, cancel := func() (context.Context, func()) {
		ctx := logr.NewContext(context.Background(), log)
		tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
		ctx, cancel := l.conn.newContext(tCtx)
		return ctx, func() { tCancel(); cancel() }
	}()
	defer cancel()

	authResp, err := authn.AuthenticateJoin(ctx, serverID, l.login.Username, optionalUserIP)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// The player disconnected before receiving we could authenticate.
			return
		}
		_ = l.conn.closeWith(packet.DisconnectWith(unableAuthWithMojang))
		return
	}

	if !authResp.OnlineMode() {
		log.Info("disconnect offline mode player")
		// Apparently an offline-mode user logged onto this online-mode proxy.
		_ = l.conn.closeWith(packet.DisconnectWith(onlineModeOnly))
		return
	}

	// Extract game profile from response.
	gameProfile, err := authResp.GameProfile()
	if err != nil {
		if l.conn.closeWith(packet.DisconnectWith(unableAuthWithMojang)) == nil {
			log.Error(err, "unable get GameProfile from Mojang authentication response")
		}
		return
	}

	// All went well, initialize the session.
	sh := newAuthSessionHandler(l.inbound, gameProfile, true)
	l.conn.setSessionHandler0(sh)
}

var (
	onlineModeOnly = &component.Text{
		Content: `This server only accepts connections from online-mode clients.

Did you change your username?
Restart your game or sign out of Minecraft, sign back in, and try again.`,
		S: component.Style{Color: color.Red},
	}
)

// Temporary english messages until localization support
var (
	alreadyConnected = &component.Text{
		Content: "You are already connected to this server!",
	}
	alreadyInProgress = &component.Text{
		Content: "You are already connecting to a server!",
	}
	noAvailableServers = &component.Text{
		Content: "No available server.", S: component.Style{Color: color.Red},
	}
	internalServerConnectionError = &component.Text{
		Content: "Internal server connection error",
	}
	// unexpectedDisconnect = &component.Text{
	//	Content: "Unexpectedly disconnected from remote server - crash?",
	// }
	movedToNewServer = &component.Text{
		Content: "The server you were on kicked you: ",
		S:       component.Style{Color: color.Red},
	}
	illegalChatCharacters = &component.Text{
		Content: "Illegal characters in chat",
		S:       component.Style{Color: color.Red},
	}
)

func (l *initialLoginSessionHandler) proxy() *Proxy {
	return l.conn.proxy
}

func (l *initialLoginSessionHandler) event() event.Manager {
	return l.proxy().event
}

func (l *initialLoginSessionHandler) config() *config.Config {
	return l.proxy().config
}

func (l *initialLoginSessionHandler) auth() auth.Authenticator {
	return l.proxy().authenticator
}

func (l *initialLoginSessionHandler) disconnected() {
	l.inbound.cleanup()
}

func (l *initialLoginSessionHandler) assertState(expectedState loginState) bool {
	if l.currentState == expectedState {
		return true
	}
	l.log.Info("received an unexpected packet during initial login session",
		"currentState", l.currentState,
		"expectedState", expectedState)
	_ = l.conn.closeKnown(true)
	return false
}

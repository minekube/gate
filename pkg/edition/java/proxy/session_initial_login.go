package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
)

type initialLoginSessionHandler struct {
	*sessionHandlerDeps

	conn    netmc.MinecraftConn
	inbound *loginInboundConn
	log     logr.Logger

	nopSessionHandler

	currentState  loginState
	originalLogin *packet.ServerLogin
	useLogin      *packet.ServerLogin
	verify        []byte
}

type loginState string

const (
	loginPacketExpectedLoginState        loginState = "loginPacketExpected"
	loginPacketReceivedLoginState        loginState = "loginPacketReceived"
	encryptionRequestSentLoginState      loginState = "encryptionRequestSent"
	encryptionResponseReceivedLoginState loginState = "encryptionResponseReceived"
)

func newInitialLoginSessionHandler(
	conn netmc.MinecraftConn,
	inbound *loginInboundConn,
	deps *sessionHandlerDeps,
) netmc.SessionHandler {
	return &initialLoginSessionHandler{
		sessionHandlerDeps: deps,
		conn:               conn,
		inbound:            inbound,
		log:                logr.FromContextOrDiscard(conn.Context()).WithName("loginSession"),
		currentState:       loginPacketExpectedLoginState,
	}
}

var invalidPlayerName = &component.Text{
	Content: "Your username has an invalid format.",
	S:       component.Style{Color: color.Red},
}

func (l *initialLoginSessionHandler) HandlePacket(p *proto.PacketContext) {
	if !p.KnownPacket() {
		// unknown packet, close connection
		_ = l.conn.Close()
		return
	}
	switch t := p.Packet.(type) {
	case *packet.ServerLogin:
		l.handleServerLogin(t)
	case *packet.LoginPluginResponse:
		_ = l.inbound.handleLoginPluginResponse(t)
	case *packet.EncryptionResponse:
		l.handleEncryptionResponse(t)
	default:
		// got unexpected packet, simply close
		_ = l.conn.Close()
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

	// Validate username format
	if !playerNameRegex.MatchString(login.Username) {
		_ = l.inbound.disconnect(invalidPlayerName)
		return
	}

	playerKey := login.PlayerKey
	if playerKey != nil {
		if playerKey.Expired() {
			l.log.V(1).Info("expired player public key")
			_ = l.inbound.disconnect(&component.Translation{
				Key: "multiplayer.disconnect.invalid_public_key_signature",
			})
			return
		}

		var isKeyValid bool
		if playerKey.KeyRevision() == keyrevision.LinkedV2 && crypto.CanSetHolder(playerKey) {
			isKeyValid = crypto.SetHolder(playerKey, login.HolderID)
		} else {
			isKeyValid = playerKey.SignatureValid()
		}

		if !isKeyValid {
			l.log.V(1).Info("invalid player public key signature")
			_ = l.inbound.disconnect(&component.Translation{
				Key: "multiplayer.disconnect.invalid_public_key",
			})
			return
		}
	} else if l.conn.Protocol().GreaterEqual(version.Minecraft_1_19) &&
		l.config().ForceKeyAuthentication &&
		l.conn.Protocol().Lower(version.Minecraft_1_19_3) {
		_ = l.inbound.disconnect(&component.Translation{
			Key: "multiplayer.disconnect.missing_public_key",
		})
		return
	}
	l.inbound.playerKey = playerKey
	l.originalLogin = login

	e := newPreLoginEvent(l.inbound, l.originalLogin.Username)
	l.eventMgr.Fire(e)

	if netmc.Closed(l.conn) {
		return // Player was disconnected
	}

	if e.Result() == DeniedPreLogin {
		_ = l.inbound.disconnect(e.Reason())
		return
	}

	l.useLogin = &packet.ServerLogin{
		Username:  e.Username(),
		PlayerKey: login.PlayerKey,
		HolderID:  login.HolderID,
	}

	_ = l.inbound.loginEventFired(func() error {
		if netmc.Closed(l.conn) {
			return nil // Player was disconnected
		}

		if e.Result() != ForceOfflineModePreLogin &&
			(e.Result() == ForceOnlineModePreLogin || l.config().OnlineMode) {

			if p, ok := netmc.Assert[GameProfileProvider](l.conn); ok {
				useProfile := p.GameProfile()
				useProfile.Name = l.useLogin.Username
				sh := l.newAuthSessionHandler(l.inbound, p.GameProfile(), useProfile, false)
				l.conn.SetSessionHandler(sh)
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
		sh := l.newAuthSessionHandler(
			l.inbound,
			profile.NewOffline(l.originalLogin.Username),
			profile.NewOffline(l.useLogin.Username),
			false)
		l.conn.SetSessionHandler(sh)
		return nil
	})
}

func (l *initialLoginSessionHandler) newAuthSessionHandler(
	inbound *loginInboundConn,
	originalProfile *profile.GameProfile,
	useProfile *profile.GameProfile,
	onlineMode bool,
) netmc.SessionHandler {
	return newAuthSessionHandler(
		inbound,
		originalProfile,
		useProfile,
		onlineMode,
		l.sessionHandlerDeps,
	)
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

	if l.originalLogin == nil {
		l.log.V(1).Info("no ServerLogin packet received yet, disconnecting")
		_ = l.conn.Close()
		return
	}
	if len(l.verify) == 0 {
		l.log.V(1).Info("no EncryptionRequest packet sent yet, disconnecting")
		_ = l.conn.Close()
		return
	}

	if playerKey := l.inbound.IdentifiedKey(); playerKey != nil {
		if resp.Salt == nil {
			l.log.V(1).Info("encryption response did not contain salt")
			_ = l.conn.Close()
			return
		}
		salt := new(bytes.Buffer)
		_ = util.WriteInt64(salt, *resp.Salt)
		valid := playerKey.VerifyDataSignature(resp.VerifyToken, l.verify, salt.Bytes())
		if !valid {
			l.log.Info("invalid client public signature")
			_ = l.conn.Close()
			return
		}
	} else {
		valid, err := l.auth().Verify(resp.VerifyToken, l.verify)
		if err != nil {
			// Simply close the connection without much overhead.
			_ = l.conn.Close()
			return
		}
		if !valid {
			l.log.Info("invalid verification token")
			_ = l.conn.Close()
			return
		}
	}

	authn := l.auth()
	decryptedSharedSecret, err := authn.DecryptSharedSecret(resp.SharedSecret)
	if err != nil {
		// Simple close the connection without much overhead.
		_ = l.conn.Close()
		return
	}

	// Enable encryption.
	// Once the client sends EncryptionResponse, encryption is enabled.
	if err = l.conn.EnableEncryption(decryptedSharedSecret); err != nil {
		l.log.Error(err, "error enabling encryption for connecting player")
		_ = netmc.CloseWith(l.conn, packet.DisconnectWith(internalServerConnectionError))
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
	ctx, cancel := context.WithTimeout(logr.NewContext(l.conn.Context(), log), 30*time.Second)
	defer cancel()

	authResp, err := authn.AuthenticateJoin(ctx, serverID, l.originalLogin.Username, optionalUserIP)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// The player disconnected before receiving we could authenticate.
			return
		}
		_ = netmc.CloseWith(l.conn, packet.DisconnectWith(unableAuthWithMojang))
		return
	}

	if !authResp.OnlineMode() {
		log.Info("disconnect offline mode player")
		// Apparently an offline-mode user logged onto this online-mode proxy.
		_ = netmc.CloseWith(l.conn, packet.DisconnectWith(onlineModeOnly))
		return
	}

	// Extract game profile from response.
	originalGameProfile, err := authResp.GameProfile()
	if err != nil {
		if netmc.CloseWith(l.conn, packet.DisconnectWith(unableAuthWithMojang)) == nil {
			log.Error(err, "unable get GameProfile from Mojang authentication response")
		}
		return
	}

	useGameProfile := originalGameProfile
	useGameProfile.Name = l.useLogin.Username

	// All went well, initialize the session.
	sh := l.newAuthSessionHandler(l.inbound, originalGameProfile, useGameProfile, true)
	l.conn.SetSessionHandler(sh)
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

func (l *initialLoginSessionHandler) Disconnected() {
	l.inbound.cleanup()
}

func (l *initialLoginSessionHandler) assertState(expectedState loginState) bool {
	if l.currentState == expectedState {
		return true
	}
	l.log.Info("received an unexpected packet during initial login session",
		"currentState", l.currentState,
		"expectedState", expectedState)
	_ = l.conn.Close()
	return false
}

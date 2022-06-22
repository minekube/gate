package proxy

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"regexp"
	"time"

	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
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
	// TODO add LoginPluginResponse & fire ServerLoginPluginMessageEvent
	case *packet.ServerLogin:
		l.handleServerLogin(t)
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
			_ = l.inbound.disconnect(&component.Translation{
				Key: "multiplayer.disconnect.invalid_public_key_signature",
			})
			return
		}

		if !playerKey.SignatureValid() {
			_ = l.inbound.disconnect(&component.Translation{
				Key: "multiplayer.disconnect.invalid_public_key",
			})
			return
		}
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

	if e.Result() != ForceOfflineModePreLogin &&
		(e.Result() == ForceOnlineModePreLogin || l.config().OnlineMode) {

		if p, ok := l.conn.c.(GameProfileProvider); ok {
			l.initPlayer(p.GameProfile(), false)
			return
		}

		// Online mode login, send encryption request
		request := l.generateEncryptionRequest()
		l.verify = make([]byte, len(request.VerifyToken))
		copy(l.verify, request.VerifyToken)
		_ = l.conn.WritePacket(request)

		// Wait for EncryptionResponse packet
		return
	}
	// Offline mode login
	l.initPlayer(profile.NewOffline(l.login.Username), false)
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
		salt := make([]byte, 8)
		binary.LittleEndian.PutUint64(salt, uint64(*resp.Salt))
		if !playerKey.VerifyDataSignature(resp.VerifyToken, l.verify, salt) {
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
	l.initPlayer(gameProfile, true)
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

func (l *initialLoginSessionHandler) initPlayer(profile *profile.GameProfile, onlineMode bool) {
	// Some connection types may need to alter the game profile.
	profile = l.conn.Type().addGameProfileTokensIfRequired(profile,
		l.proxy().Config().Forwarding.Mode)

	profileRequest := NewGameProfileRequestEvent(l.inbound, *profile, onlineMode)
	l.proxy().event.Fire(profileRequest)
	if l.conn.Closed() {
		return // Player disconnected after authentication
	}
	gameProfile := profileRequest.GameProfile()

	// Initiate a regular connection and move over to it.
	player := newConnectedPlayer(l.conn, &gameProfile,
		l.inbound.VirtualHost(), onlineMode, l.inbound.IdentifiedKey())
	if !l.proxy().canRegisterConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	l.log.Info("Player has connected, completing login", "player", player, "id", player.ID())

	// Setup permissions
	permSetup := &PermissionsSetupEvent{
		subject:     player,
		defaultFunc: player.permFunc,
	}
	player.proxy.event.Fire(permSetup)
	// Set the player's permission function
	player.permFunc = permSetup.Func()

	if player.Active() {
		l.completeLoginProtocolPhaseAndInit(player)
	}
}

func (l *initialLoginSessionHandler) completeLoginProtocolPhaseAndInit(player *connectedPlayer) {
	cfg := l.config()

	// Send compression threshold
	threshold := cfg.Compression.Threshold
	if threshold >= 0 && player.Protocol().GreaterEqual(version.Minecraft_1_8) {
		err := player.WritePacket(&packet.SetCompression{Threshold: threshold})
		if err != nil {
			_ = player.close()
			return
		}
		if err = player.SetCompressionThreshold(threshold); err != nil {
			l.log.Error(err, "Error setting compression threshold")
			_ = l.inbound.disconnect(internalServerConnectionError))
			return
		}
	}

	// Send login success
	playerID := player.ID()
	if cfg.Forwarding.Mode == config.NoneForwardingMode {
		playerID = uuid.OfflinePlayerUUID(player.Username())
	}
	if player.WritePacket(&packet.ServerLoginSuccess{
		UUID:     playerID,
		Username: player.Username(),
	}) != nil {
		return
	}

	player.setState(state.Play)
	loginEvent := &LoginEvent{player: player}
	l.event().Fire(loginEvent)

	if !player.Active() {
		l.event().Fire(&DisconnectEvent{
			player:      player,
			loginStatus: CanceledByUserBeforeCompleteLoginStatus,
		})
		return
	}

	if !loginEvent.Allowed() {
		player.Disconnect(loginEvent.Reason())
		return
	}

	if !l.proxy().registerConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	// Login is done now, just connect player to first server and
	// let InitialConnectSessionHandler do further work.
	player.setSessionHandler(newInitialConnectSessionHandler(player))
	l.event().Fire(&PostLoginEvent{player: player})
	l.connectToInitialServer(player)
}

func (l *initialLoginSessionHandler) connectToInitialServer(player *connectedPlayer) {
	initialFromConfig := player.nextServerToTry(nil)
	chooseServer := &PlayerChooseInitialServerEvent{
		player:        player,
		initialServer: initialFromConfig,
	}
	l.event().Fire(chooseServer)
	if !player.Active() || // player was disconnected
		player.CurrentServer() != nil { // player was already connected to a server
		return
	}
	if chooseServer.InitialServer() == nil {
		player.Disconnect(noAvailableServers) // Will call disconnected() in InitialConnectSessionHandler
		return
	}
	ctx, cancel := withConnectionTimeout(context.Background(), l.config())
	defer cancel()
	ctx, pcancel := player.newContext(ctx) // todo use player's connection context
	defer pcancel()
	player.CreateConnectionRequest(chooseServer.InitialServer()).ConnectWithIndication(ctx)
}

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

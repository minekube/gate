package proxy

import (
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type authSessionHandler struct {
	*sessionHandlerDeps

	log        logr.Logger
	inbound    *loginInboundConn
	profile    *profile.GameProfile
	onlineMode bool

	loginState *atomic.Pointer[authLoginState] // 1.20.2+

	connectedPlayer *connectedPlayer
}

type authLoginState int

var (
	startAuthLoginState        authLoginState = 0
	successSentAuthLoginState  authLoginState = 1
	acknowledgedAuthLoginState authLoginState = 2
)

type playerRegistrar interface {
	canRegisterConnection(player *connectedPlayer) bool
	registerConnection(player *connectedPlayer) bool
	unregisterConnection(player *connectedPlayer) bool
}

func newAuthSessionHandler(
	inbound *loginInboundConn,
	profile *profile.GameProfile,
	onlineMode bool,
	sessionHandlerDeps *sessionHandlerDeps,
) netmc.SessionHandler {
	var defaultState atomic.Pointer[authLoginState]
	defaultState.Store(&startAuthLoginState)
	return &authSessionHandler{
		loginState:         &defaultState,
		sessionHandlerDeps: sessionHandlerDeps,
		log:                logr.FromContextOrDiscard(inbound.Context()).WithName("authSession"),
		inbound:            inbound,
		profile:            profile,
		onlineMode:         onlineMode,
	}
}

func (a *authSessionHandler) Disconnected() {
	defer a.inbound.cleanup()
	if a.connectedPlayer != nil {
		// Clear loginInbound to avoid memory leaks
		a.connectedPlayer.clearLoginInbound()
		a.connectedPlayer.teardown()
	}
}

func (a *authSessionHandler) Activated() {
	// Some connection types may need to alter the game profile.
	gameProfile := *a.inbound.delegate.Type().AddGameProfileTokensIfRequired(
		a.profile, a.config().Forwarding.Mode)
	profileRequest := NewGameProfileRequestEvent(a.inbound, gameProfile, a.onlineMode)
	a.eventMgr.Fire(profileRequest)
	conn := a.inbound.delegate.MinecraftConn
	if netmc.Closed(conn) {
		return // Player disconnected after authentication
	}
	gameProfile = profileRequest.GameProfile()

	// Initiate a regular connection and move over to it.
	player := newConnectedPlayer(
		conn,
		&gameProfile,
		a.inbound.VirtualHost(),
		a.inbound.HandshakeIntent(),
		a.onlineMode,
		a.inbound.IdentifiedKey(),
		a.sessionHandlerDeps,
	)
	a.connectedPlayer = player

	// For Modern Forge clients, store the loginInbound for login plugin message forwarding.
	// This allows the backend login handler to forward Forge handshake messages to the client.
	if conn.Type() == phase.ModernForge {
		player.setLoginInbound(a.inbound)
		a.log.V(1).Info("stored loginInbound for Modern Forge login plugin forwarding")
	}

	if !a.registrar.canRegisterConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	a.log.Info("player has connected, completing login", "player", player, "id", player.ID())

	// Setup permissions
	permSetup := &PermissionsSetupEvent{
		subject:     player,
		defaultFunc: player.permFunc,
	}
	a.eventMgr.Fire(permSetup)
	// Set the player's permission function
	player.permFunc = permSetup.Func()

	if player.Active() {
		a.startLoginCompletion(player)
	}
}

func (a *authSessionHandler) startLoginCompletion(player *connectedPlayer) {
	cfg := a.config()

	// Send compression threshold
	threshold := cfg.Compression.Threshold
	if threshold >= 0 && player.Protocol().GreaterEqual(version.Minecraft_1_8) {
		err := player.WritePacket(&packet.SetCompression{Threshold: threshold})
		if err != nil {
			_ = player.Close()
			return
		}
		if err = player.SetCompressionThreshold(threshold); err != nil {
			a.log.Error(err, "Error setting compression threshold")
			_ = a.inbound.disconnect(internalServerConnectionError)
			return
		}
	}

	// Send login success
	playerID := player.ID()
	if cfg.Forwarding.Mode == config.NoneForwardingMode {
		playerID = uuid.OfflinePlayerUUID(player.Username())
	}

	if playerKey := player.IdentifiedKey(); playerKey != nil {
		if playerKey.SignatureHolder() == uuid.Nil {
			// Failsafe
			if !crypto.SetHolder(playerKey, playerID) {
				if a.onlineMode {
					_ = a.inbound.disconnect(&component.Translation{
						Key: "multiplayer.disconnect.invalid_public_key",
					})
					return
				}
				a.log.Info("key for player could not be verified", "player", player.Username())
			}
		} else {
			if playerKey.SignatureHolder() != playerID {
				a.log.Info("uuid for player mismatches, "+
					"chat/commands signatures will not work correctly for this player",
					"player", player.Username())
			}
		}
	}

	// For Modern Forge clients, we need to connect to the backend BEFORE sending
	// ServerLoginSuccess, so that login plugin messages can be forwarded while
	// the client is still in LOGIN state.
	if player.Type() == phase.ModernForge && a.inbound.Protocol().Lower(version.Minecraft_1_20_2) {
		a.log.Info("Modern Forge client detected, deferring login completion until backend login")
		a.startModernForgeLogin(player)
		return
	}

	a.completeLoginProtocolPhaseAndInitialize(player)
}

// startModernForgeLogin initiates backend connection for Modern Forge clients
// while keeping the client in LOGIN state for login plugin message forwarding.
func (a *authSessionHandler) startModernForgeLogin(player *connectedPlayer) {
	loginEvent := &LoginEvent{player: player}
	a.eventMgr.Fire(loginEvent)
	if !player.Active() {
		a.eventMgr.Fire(&DisconnectEvent{
			player:      player,
			loginStatus: CanceledByUserBeforeCompleteLoginStatus,
		})
		return
	}

	if !loginEvent.Allowed() {
		player.Disconnect(loginEvent.Reason())
		return
	}

	if !a.registrar.registerConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	// Fire PostLoginEvent early so plugins can interact
	a.eventMgr.Fire(&PostLoginEvent{player: player})

	// Choose initial server
	initialFromConfig := player.nextServerToTry(nil)
	chooseServer := &PlayerChooseInitialServerEvent{
		player:        player,
		initialServer: initialFromConfig,
	}
	a.eventMgr.Fire(chooseServer)
	if !player.Active() {
		return
	}
	if chooseServer.InitialServer() == nil {
		player.Disconnect(noAvailableServers)
		return
	}

	// Connect to backend server while client is still in LOGIN state
	// The backendLoginSessionHandler will forward login plugin messages
	// via the stored loginInbound
	a.log.Info("connecting to backend for Modern Forge handshake",
		"server", chooseServer.InitialServer().ServerInfo().Name())

	// Enable immediate sending of login plugin messages (don't queue them)
	a.inbound.enableImmediateSend()

	ctx, cancel := withConnectionTimeout(player.Context(), a.config())
	defer cancel()

	// Create connection request and connect
	result, err := player.CreateConnectionRequest(chooseServer.InitialServer()).Connect(ctx)
	if err != nil {
		a.log.Error(err, "failed to connect to backend for Modern Forge handshake")
		player.Disconnect(&component.Text{Content: "Failed to connect to server: " + err.Error()})
		return
	}

	if !result.Status().Successful() {
		reason := result.Reason()
		if reason == nil {
			reason = &component.Text{Content: "Connection failed"}
		}
		a.log.Info("backend connection failed for Modern Forge handshake",
			"status", result.Status(), "reason", reason)
		player.Disconnect(reason)
		return
	}

	// Backend login succeeded! Now complete client login.
	a.log.Info("backend login successful, completing client login for Modern Forge")

	// Send ServerLoginSuccess to client
	if player.WritePacket(&packet.ServerLoginSuccess{
		UUID:       player.ID(),
		Username:   player.Username(),
		Properties: player.GameProfile().Properties,
	}) != nil {
		return
	}

	a.loginState.Store(&successSentAuthLoginState)
	a.loginState.Store(&acknowledgedAuthLoginState)

	// Transition client to PLAY state
	a.connectedPlayer.MinecraftConn.SetActiveSessionHandler(state.Play,
		newInitialConnectSessionHandler(a.connectedPlayer))

	// Clear loginInbound since we're done with login phase
	player.clearLoginInbound()
}

func (a *authSessionHandler) completeLoginProtocolPhaseAndInitialize(player *connectedPlayer) {
	loginEvent := &LoginEvent{player: player}
	// should fire event in sync to retain unlocked decoder to update state
	a.eventMgr.Fire(loginEvent)
	if !player.Active() {
		a.eventMgr.Fire(&DisconnectEvent{
			player:      player,
			loginStatus: CanceledByUserBeforeCompleteLoginStatus,
		})
		return
	}

	if !loginEvent.Allowed() {
		player.Disconnect(loginEvent.Reason())
		return
	}

	if !a.registrar.registerConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	if player.WritePacket(&packet.ServerLoginSuccess{
		UUID:       player.ID(),
		Username:   player.Username(),
		Properties: player.GameProfile().Properties,
	}) != nil {
		return
	}

	a.loginState.Store(&successSentAuthLoginState)

	if a.inbound.Protocol().Lower(version.Minecraft_1_20_2) {
		a.loginState.Store(&acknowledgedAuthLoginState)
		a.connectedPlayer.MinecraftConn.SetActiveSessionHandler(state.Play,
			newInitialConnectSessionHandler(a.connectedPlayer))

		a.eventMgr.Fire(&PostLoginEvent{player: player})
		a.connectToInitialServer(player)
	}
}

// connectToInitialServer connects the player to the initial server as per the player's information.
// If the player is active and not already connected to a server, the connection is initiated.
// If no initial server is found, the player is disconnected.
// This function is primarily used during the player login process.
func (a *authSessionHandler) connectToInitialServer(player *connectedPlayer) {
	initialFromConfig := player.nextServerToTry(nil)
	chooseServer := &PlayerChooseInitialServerEvent{
		player:        player,
		initialServer: initialFromConfig,
	}
	a.eventMgr.Fire(chooseServer)
	if !player.Active() || // player was disconnected
		player.CurrentServer() != nil { // player was already connected to a server
		return
	}
	if chooseServer.InitialServer() == nil {
		player.Disconnect(noAvailableServers) // Will call Disconnected() in InitialConnectSessionHandler
		return
	}
	ctx, cancel := withConnectionTimeout(player.Context(), a.config())
	defer cancel()
	player.CreateConnectionRequest(chooseServer.InitialServer()).ConnectWithIndication(ctx)
}

func (a *authSessionHandler) Deactivated() {}

func (a *authSessionHandler) HandlePacket(pc *proto.PacketContext) {
	a.log.V(1).Info("received packet in auth session",
		"packetType", fmt.Sprintf("%T", pc.Packet),
		"packetID", pc.PacketID,
		"known", pc.KnownPacket())

	switch t := pc.Packet.(type) {
	case *packet.LoginAcknowledged:
		a.handleLoginAcknowledged()
	case *cookie.CookieResponse:
		a.handleCookieResponse(t)
	case *packet.LoginPluginResponse:
		// Handle login plugin response for Modern Forge forwarding
		a.log.Info("received LoginPluginResponse", "id", t.ID, "success", t.Success, "dataLen", len(t.Data))
		// First try Forge direct forwarding
		if a.inbound.handleForgeLoginPluginResponse(t) {
			a.log.V(1).Info("handled as Forge response")
			return
		}
		// Fall back to normal handling
		if err := a.inbound.handleLoginPluginResponse(t); err != nil {
			a.log.Error(err, "error handling login plugin response")
		}
	default:
		a.log.Info("unexpected packet during auth session",
			"packet", pc.Packet,
			"packet_id", pc.PacketID,
			"player", a.connectedPlayer.String(),
		)
		_ = a.inbound.delegate.Close()
	}

}

func (a *authSessionHandler) config() *config.Config {
	return a.configProvider.config()
}

func (a *authSessionHandler) handleLoginAcknowledged() bool {
	if *a.loginState.Load() != successSentAuthLoginState {
		_ = a.inbound.disconnect(&component.Translation{
			Key: "multiplayer.disconnect.invalid_player_data",
		})
	} else {
		a.loginState.Store(&acknowledgedAuthLoginState)
		a.connectedPlayer.MinecraftConn.SetActiveSessionHandler(state.Config,
			newClientConfigSessionHandler(a.connectedPlayer))

		event.FireParallel(a.eventMgr, &PostLoginEvent{player: a.connectedPlayer}, func(postLoginEvent *PostLoginEvent) {
			if !a.connectedPlayer.Active() {
				return
			}
			a.connectToInitialServer(a.connectedPlayer)
		})
	}
	return true
}

func (a *authSessionHandler) handleCookieResponse(p *cookie.CookieResponse) {
	e := newCookieReceiveEvent(a.connectedPlayer, p.Key, p.Payload)
	a.eventMgr.Fire(e)
	if e.Allowed() {
		// The received cookie must have been requested by a proxy plugin in login phase,
		// because if a backend server requests a cookie in login phase, the client is already
		// in config phase. Therefore, the only way, we receive a CookieResponsePacket from a
		// client in login phase is when a proxy plugin requested a cookie in login phase.
		a.log.Info("a cookie was requested by a proxy plugin in login phase but the response wasn't handled")
	}
}

package proxy

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/color"
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
	"go.minekube.com/gate/pkg/internal/identity"
	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
	"go.minekube.com/gate/pkg/util/uuid"
)

type authSessionHandler struct {
	*sessionHandlerDeps

	log          logr.Logger
	inbound      *loginInboundConn
	profile      *profile.GameProfile
	onlineMode   bool
	supplied     suppliedIdentityTrust
	serverIDHash string

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
	supplied suppliedIdentityTrust,
	serverIDHash string,
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
		supplied:           supplied,
		serverIDHash:       serverIDHash,
	}
}

func (a *authSessionHandler) Disconnected() {
	defer a.inbound.cleanup()
	observeAuthDisconnect(a.inbound.Context())
	if a.connectedPlayer != nil {
		a.connectedPlayer.teardown()
	}
}

func observeAuthDisconnect(ctx context.Context) {
	if observation, ok := connectiontelemetry.FromContext(ctx); ok {
		// Until PLAY takes over, any client-side auth disconnect/reject is a
		// failed login. Session terminal de-duplication preserves a timeout if
		// the reader already recorded that more specific outcome.
		observation.Observe(ctx, connectiontelemetry.Closed, connectiontelemetry.Failed)
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

	// Assign the UUID this player is already known by on the backend servers.
	// This has to run after the profile request event (the last chance to
	// change the profile) and before newConnectedPlayer, because every
	// consumer downstream - registry, duplicate detection, LoginSuccess,
	// forwarding payloads, LoginStart HolderID, plugin API - reads
	// connectedPlayer.ID().
	store := a.proxy.identityStore
	var identitySource identity.Source
	if store != nil {
		source, ok := a.assignStoredIdentity(store, &gameProfile)
		if !ok {
			return // login rejected, the reason was already sent
		}
		identitySource = source
	}

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
	if !a.registrar.canRegisterConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	// Log the identity the player finally has on the backend server. It is the
	// profile UUID Gate forwards with player info forwarding enabled, and the
	// backend-derived offline UUID when forwarding is disabled, so the same
	// account can end up with different UUIDs across configurations.
	//
	// Only the identity store gives a login a UUID other than the one it
	// resolved to, so these fields are logged only while it persists player
	// data; without it the line stays the plain player and id.
	logFields := []any{"player", player, "id", player.ID()}
	if store != nil {
		forwarding := a.config().Forwarding.Mode
		logFields = append(logFields,
			"backendUid", backendPlayerID(forwarding, player),
			"onlineMode", a.onlineMode,
			"forwarding", forwarding,
			"identitySource", string(identitySource),
			"keyHolder", keyHolderOf(a.inbound.IdentifiedKey()),
		)
	}
	a.log.Info("player has connected, completing login", logFields...)

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

// identityStoreTimeout bounds how long a login waits for the identity store.
// It outlasts the store's own busy timeout, so a busy database fails the login
// with a store error instead of a context deadline.
const identityStoreTimeout = 10 * time.Second

// identityStoreUnavailable is sent to a player whose identity cannot be read.
var identityStoreUnavailable = &component.Text{
	Content: "Unable to load your player identity.\nPlease try again!",
	S:       component.Style{Color: color.Red},
}

// premiumAccountRequired is sent to an offline login for a username whose
// account has been authenticated before.
var premiumAccountRequired = &component.Text{
	Content: "This username is protected.\n" +
		"Please log in with the Minecraft account that owns it.",
	S: component.Style{Color: color.Red},
}

// suppliedIdentityTrust says what is known about an identity a connection
// supplied instead of the proxy authenticating the login itself.
//
// The zero value is the one to assume by default: a login only gets more than
// that when the connection declares that its endpoint carries only authenticated
// players.
type suppliedIdentityTrust int

const (
	// unvouchedSuppliedIdentity is an identity nothing vouched for. The proxy
	// knows no more than that its UUID is not the username's offline UUID.
	unvouchedSuppliedIdentity suppliedIdentityTrust = iota
	// vouchedSuppliedIdentity is an identity its ingress vouched for: the
	// endpoint declared it does not accept offline-mode players, so the tunnel
	// service only proposes authenticated players here. The proxy still did not
	// verify the identity itself.
	vouchedSuppliedIdentity
)

// resolvedIdentitySource classifies the identity the login flow produced, which
// decides the UUID the store records for the account.
//
// An offline-mode identity is one whose UUID is the digest of the username, and
// it is recognized before the vouched case: an identity that is the username's
// offline UUID is an offline identity no matter which ingress supplied it, so a
// vouch can never turn one into an authenticated account. Any other UUID was
// supplied by an authenticated login or by a connection type such as Geyser or
// a Connect tunnel.
func resolvedIdentitySource(onlineMode bool, supplied suppliedIdentityTrust, p profile.GameProfile) identity.Source {
	switch {
	case onlineMode:
		return identity.SourcePremium
	case p.Name != "" && p.ID == uuid.OfflinePlayerUUID(p.Name):
		return identity.SourceOffline
	case supplied == vouchedSuppliedIdentity:
		return identity.SourceConnectAuthenticated
	default:
		return identity.SourceInjected
	}
}

// keyHolderOf returns the UUID the player's profile public key is signed for,
// or an empty string if there is no such key. A key holder that differs from
// the assigned UUID is what breaks chat signatures on the backend.
func keyHolderOf(key crypto.IdentifiedKey) string {
	if key == nil || key.SignatureHolder() == uuid.Nil {
		return ""
	}
	return key.SignatureHolder().String()
}

// assignStoredIdentity replaces the profile UUID with the one the player is
// already known by on the backend servers, registering the account on its first
// login. It reports whether the login may continue; when it returns false the
// player has already been disconnected.
func (a *authSessionHandler) assignStoredIdentity(
	store *identity.Store,
	gameProfile *profile.GameProfile,
) (identity.Source, bool) {
	ctx, cancel := context.WithTimeout(a.inbound.Context(), identityStoreTimeout)
	defer cancel()

	resolved := gameProfile.ID
	source := resolvedIdentitySource(a.onlineMode, a.supplied, *gameProfile)
	result, err := store.Resolve(ctx, gameProfile.Name, resolved, source)
	if err != nil {
		if a.config().IdentityStore.FailOpen {
			a.log.Error(err, "error reading the player identity store, keeping the resolved UUID",
				"player", gameProfile.Name, "id", resolved)
			return source, true
		}
		a.log.Error(err, "error reading the player identity store, rejecting login",
			"player", gameProfile.Name)
		_ = a.inbound.disconnect(identityStoreUnavailable)
		return source, false
	}

	// A protected account only accepts an authenticated login, so no cracked
	// login path can take it over. See premiumProtection.
	if protection := newPremiumProtection(a.config()); !protection.allows(gameProfile.Name, resolved, result.Record, source) {
		a.log.Info("refusing a login for a protected account",
			"player", gameProfile.Name, "id", result.ActualID, "authenticated", result.Authenticated,
			"source", string(source), "mode", string(protection.mode))
		_ = a.inbound.disconnect(premiumAccountRequired)
		return source, false
	}

	switch {
	case result.Created:
		a.log.Info("registered a new player identity",
			"player", gameProfile.Name, "id", result.ActualID, "source", string(source))
	case result.ActualID != resolved:
		a.log.Info("assigned the account's bound UUID",
			"player", gameProfile.Name, "resolved", resolved, "id", result.ActualID,
			"source", string(source), "match", string(result.Match))
	}

	gameProfile.ID = result.ActualID
	// Report how this login resolved; the account's bound UUID and whether it is
	// authenticated are logged by the store lines above.
	return source, true
}

// backendPlayerID returns the UUID the player finally has on a backend server.
//
// Gate forwards the player's profile UUID in every forwarding mode but "none":
// the Velocity forwarding payload, the legacy/BungeeGuard handshake and the
// LoginStart HolderID all carry it. With forwarding disabled nothing is
// forwarded, so the backend derives the offline UUID from the username itself
// and that derived UUID is the one the player is known by there.
func backendPlayerID(mode config.ForwardingMode, player *connectedPlayer) uuid.UUID {
	if mode == config.NoneForwardingMode {
		return uuid.OfflinePlayerUUID(player.Username())
	}
	return player.ID()
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
	playerID := backendPlayerID(cfg.Forwarding.Mode, player)

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

	a.completeLoginProtocolPhaseAndInitialize(player)
}

func (a *authSessionHandler) completeLoginProtocolPhaseAndInitialize(player *connectedPlayer) {
	loginEvent := &LoginEvent{player: player, serverIDHash: a.serverIDHash}
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

	loginSuccess := &packet.ServerLoginSuccess{
		UUID:       player.ID(),
		Username:   player.Username(),
		Properties: player.GameProfile().Properties,
	}
	if a.inbound.Protocol().GreaterEqual(version.Minecraft_26_2) {
		loginSuccess.SessionID = a.proxy.sessionID()
	}

	// For Modern Forge clients on pre-1.20.2, delay sending LoginSuccess so the
	// client remains in the LOGIN state during the backend FML handshake relay.
	// The relay will send LoginSuccess after the FML negotiation completes.
	//
	// We run connectToInitialServer in a goroutine so the client's read loop can
	// process LoginPluginResponse packets for the FML relay. The decoder's SetState
	// uses atomic pointer swaps, so switching the client to PLAY from the backend
	// goroutine (in handleServerLoginSuccess) does not block on the decoder mutex.
	if a.inbound.Protocol().Lower(version.Minecraft_1_20_2) && player.Type() == phase.ModernForge {
		a.log.Info("delaying LoginSuccess for Modern Forge FML handshake relay")
		relay := newModernForgeLoginRelay(a.inbound, player, loginSuccess)
		player.mu.Lock()
		player.forgeLoginRelay = relay
		player.mu.Unlock()

		a.inbound.clearOnAllMessagesHandled()
		a.loginState.Store(&acknowledgedAuthLoginState)

		// Connect in a goroutine so the client's read loop can process
		// LoginPluginResponse packets for the FML relay.
		go a.connectToInitialServer(player)
		return
	}

	if player.WritePacket(loginSuccess) != nil {
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
	player.createInitialConnectionRequest(chooseServer.InitialServer()).ConnectWithIndication(ctx)
}

func (a *authSessionHandler) Deactivated() {}

func (a *authSessionHandler) HandlePacket(pc *proto.PacketContext) {
	switch t := pc.Packet.(type) {
	case *packet.LoginAcknowledged:
		a.handleLoginAcknowledged()
	case *packet.LoginPluginResponse:
		a.handleLoginPluginResponse(t)
	case *cookie.CookieResponse:
		a.handleCookieResponse(t)
	default:
		a.log.Info("unexpected packet during auth session",
			"packet", pc.Packet,
			"packet_id", pc.PacketID,
			"player", a.connectedPlayer.String(),
		)
		_ = a.inbound.delegate.Close()
	}

}

func (a *authSessionHandler) handleLoginPluginResponse(p *packet.LoginPluginResponse) {
	if err := a.inbound.handleLoginPluginResponse(p); err != nil {
		a.log.Error(err, "error handling login plugin response during forge relay")
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

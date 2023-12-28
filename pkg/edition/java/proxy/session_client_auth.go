package proxy

import (
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type authSessionHandler struct {
	*sessionHandlerDeps

	log        logr.Logger
	inbound    *loginInboundConn
	profile    *profile.GameProfile
	onlineMode bool

	loginState authLoginState // 1.20.2+

	connectedPlayer *connectedPlayer
}

type authLoginState int

const (
	startAuthLoginState authLoginState = iota
	successSentAuthLoginState
	acknowledgedAuthLoginState
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
	return &authSessionHandler{
		loginState:         startAuthLoginState,
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
		a.onlineMode,
		a.inbound.IdentifiedKey(),
		a.sessionHandlerDeps,
	)
	a.connectedPlayer = player
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

	a.completeLoginProtocolPhaseAndInitialize(player)
}

func (a *authSessionHandler) completeLoginProtocolPhaseAndInitialize(player *connectedPlayer) {
	event.FireParallel(a.eventMgr, &LoginEvent{player: player}, func(loginEvent *LoginEvent) {
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

		a.loginState = successSentAuthLoginState

		if a.inbound.Protocol().Lower(version.Minecraft_1_20_2) {
			a.loginState = acknowledgedAuthLoginState
			a.connectedPlayer.MinecraftConn.SetActiveSessionHandler(state.Play,
				newInitialConnectSessionHandler(a.connectedPlayer))

			a.eventMgr.Fire(&PostLoginEvent{player: player})
			a.connectToInitialServer(player)
		}
	})
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
	switch pc.Packet.(type) {
	case *packet.LoginAcknowledged:
		a.handleLoginAcknowledged()
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
	if a.loginState != successSentAuthLoginState {
		_ = a.inbound.disconnect(&component.Translation{
			Key: "multiplayer.disconnect.invalid_player_data",
		})
	} else {
		a.loginState = acknowledgedAuthLoginState
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

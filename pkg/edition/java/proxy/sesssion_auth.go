package proxy

import (
	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/util/uuid"
)

type authSessionHandler struct {
	log        logr.Logger
	inbound    *loginInboundConn
	profile    *profile.GameProfile
	onlineMode bool

	connectedPlayer *connectedPlayer
}

func newAuthSessionHandler(inbound *loginInboundConn, profile *profile.GameProfile, onlineMode bool) sessionHandler {
	return &authSessionHandler{
		log:        inbound.delegate.log.WithName("authSession"),
		inbound:    inbound,
		profile:    profile,
		onlineMode: onlineMode,
	}
}

func (a *authSessionHandler) disconnected() {
	defer a.inbound.cleanup()
	if a.connectedPlayer != nil {
		a.connectedPlayer.teardown()
	}
}

func (a *authSessionHandler) activated() {
	// Some connection types may need to alter the game profile.
	gameProfile := *a.inbound.delegate.Type().addGameProfileTokensIfRequired(a.profile,
		a.proxy().Config().Forwarding.Mode)

	profileRequest := NewGameProfileRequestEvent(a.inbound, gameProfile, a.onlineMode)
	a.event().Fire(profileRequest)
	if a.inbound.delegate.minecraftConn.Closed() {
		return // Player disconnected after authentication
	}
	gameProfile = profileRequest.GameProfile()

	// Initiate a regular connection and move over to it.
	player := newConnectedPlayer(a.inbound.delegate.minecraftConn, &gameProfile,
		a.inbound.VirtualHost(), a.onlineMode, a.inbound.IdentifiedKey())
	a.connectedPlayer = player
	if !a.proxy().canRegisterConnection(player) {
		player.Disconnect(alreadyConnected)
		return
	}

	a.log.Info("player has connected, completing login", "player", player, "id", player.ID())

	// Setup permissions
	permSetup := &PermissionsSetupEvent{
		subject:     player,
		defaultFunc: player.permFunc,
	}
	a.event().Fire(permSetup)
	// Set the player's permission function
	player.permFunc = permSetup.Func()

	if player.Active() {
		a.completeLoginProtocolPhaseAndInit(player)
	}
}

func (a *authSessionHandler) completeLoginProtocolPhaseAndInit(player *connectedPlayer) {
	cfg := a.proxy().config

	// Send compression threshold
	threshold := cfg.Compression.Threshold
	if threshold >= 0 && player.Protocol().GreaterEqual(version.Minecraft_1_8) {
		err := player.WritePacket(&packet.SetCompression{Threshold: threshold})
		if err != nil {
			_ = player.close()
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
		if playerKey.SignatureHolder() != uuid.Nil {
			if playerKey
		} else {

		}
	}

	if player.WritePacket(&packet.ServerLoginSuccess{
		UUID:       playerID,
		Username:   player.Username(),
		Properties: player.GameProfile().Properties,
	}) != nil {
		return
	}

	player.setState(state.Play)
	loginEvent := &LoginEvent{player: player}
	a.event().FireParallel(loginEvent, func(ev event.Event) {
		loginEvent = ev.(*LoginEvent)

		if !player.Active() {
			a.event().Fire(&DisconnectEvent{
				player:      player,
				loginStatus: CanceledByUserBeforeCompleteLoginStatus,
			})
			return
		}

		if !loginEvent.Allowed() {
			player.Disconnect(loginEvent.Reason())
			return
		}

		if !a.proxy().registerConnection(player) {
			player.Disconnect(alreadyConnected)
			return
		}

		// Login is done now, just connect player to first server and
		// let InitialConnectSessionHandler do further work.
		player.setSessionHandler(newInitialConnectSessionHandler(player))
		a.event().Fire(&PostLoginEvent{player: player})
		a.connectToInitialServer(player)
	})
}

func (a *authSessionHandler) connectToInitialServer(player *connectedPlayer) {
	initialFromConfig := player.nextServerToTry(nil)
	chooseServer := &PlayerChooseInitialServerEvent{
		player:        player,
		initialServer: initialFromConfig,
	}
	a.event().Fire(chooseServer)
	if !player.Active() || // player was disconnected
		player.CurrentServer() != nil { // player was already connected to a server
		return
	}
	if chooseServer.InitialServer() == nil {
		player.Disconnect(noAvailableServers) // Will call disconnected() in InitialConnectSessionHandler
		return
	}
	ctx, cancel := withConnectionTimeout(player.Context(), a.proxy().config)
	defer cancel()
	player.CreateConnectionRequest(chooseServer.InitialServer()).ConnectWithIndication(ctx)
}

func (a *authSessionHandler) deactivated() {}

func (a *authSessionHandler) handlePacket(pc *proto.PacketContext) {
	// no packet expected during auth session
	_ = a.inbound.delegate.closeKnown(true)
}

func (a *authSessionHandler) proxy() *Proxy        { return a.inbound.delegate.proxy }
func (a *authSessionHandler) event() event.Manager { return a.proxy().Event() }

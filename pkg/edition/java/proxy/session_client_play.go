package proxy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/internal/future"

	"github.com/gammazero/deque"
	"go.minekube.com/common/minecraft/component"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/bossbar"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/util/uuid"

	"github.com/go-logr/logr"

	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/validation"
)

// Handles communication with the connected Minecraft client.
// This is effectively the primary nerve center that joins backend servers with players.
type clientPlaySessionHandler struct {
	log, log1              logr.Logger
	player                 *connectedPlayer
	spawned                atomic.Bool
	chatHandler            *chatHandler
	chatTimeKeeper         chatTimeKeeper
	outstandingTabComplete *packet.TabCompleteRequest

	configSwitchFuture future.Future[any]

	mu struct {
		sync.RWMutex
		loginPluginMessages deque.Deque[*plugin.Message]
		serverBossBars      map[uuid.UUID]struct{}
	}
}

func newClientPlaySessionHandler(player *connectedPlayer) *clientPlaySessionHandler {
	log := player.log.WithName("clientPlaySession")
	h := &clientPlaySessionHandler{
		player: player,
		log:    log,
		log1:   log.V(1),
		chatHandler: &chatHandler{
			log:            log,
			eventMgr:       player.eventMgr,
			player:         player,
			cmdMgr:         player.proxy.Command(),
			configProvider: player.proxy,
		},
	}
	h.mu.serverBossBars = map[uuid.UUID]struct{}{}
	return h
}

var _ netmc.SessionHandler = (*clientPlaySessionHandler)(nil)

func (c *clientPlaySessionHandler) HandlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket() {
		c.forwardToServer(pc)
		return
	}

	switch p := pc.Packet.(type) {
	case *packet.KeepAlive:
		c.handleKeepAlive(p)
	case *chat.LegacyChat:
		c.handleLegacyChat(p)
	case *chat.SessionPlayerChat:
		c.handleSessionPlayerChat(p)
	case *chat.KeyedPlayerChat:
		c.handleKeyedPlayerChat(p)
	case *chat.KeyedPlayerCommand:
		c.handleKeyedPlayerCommand(p)
	case *chat.SessionPlayerCommand:
		c.handleSessionPlayerCommand(p)
	case *chat.UnsignedPlayerCommand:
		c.handleSessionPlayerCommand(&p.SessionPlayerCommand)
	case *packet.TabCompleteRequest:
		c.handleTabCompleteRequest(p, pc)
	case *plugin.Message:
		c.handlePluginMessage(p)
	case *packet.ResourcePackResponse:
		if !handleResourcePackResponse(p, c.player.resourcePackHandler, c.log) {
			forwardToServer(pc, c.player)
		}
	case *packet.ClientSettings:
		c.player.setClientSettings(p)
		c.forwardToServer(pc) // forward to server
	case *config.FinishedUpdate:
		c.handleFinishedUpdate(p)
	case *chat.ChatAcknowledgement:
		c.handleChatAcknowledgement(p)
	case *cookie.CookieResponse:
		c.handleCookieResponse(p)
	default:
		c.forwardToServer(pc)
	}
}

func (c *clientPlaySessionHandler) Deactivated() {
	c.mu.Lock()
	c.mu.loginPluginMessages.Clear()
	c.mu.Unlock()
}

func (c *clientPlaySessionHandler) Activated() {
	protocol := c.player.Protocol()
	channels := c.player.proxy.ChannelRegistrar().ChannelsForProtocol(protocol)
	if len(channels) != 0 {
		register := plugin.ConstructChannelsPacket(protocol, channels.UnsortedList()...)
		_ = c.player.WritePacket(register)
	}
}

func (c *clientPlaySessionHandler) forwardToServer(pc *proto.PacketContext) {
	forwardToServer(pc, c.player)
}

func forwardToServer(pc *proto.PacketContext, player *connectedPlayer) {
	if serverMc := canForward(player); serverMc != nil {
		_ = serverMc.Write(pc.Payload)
	}
}

func canForward(player *connectedPlayer) netmc.MinecraftConn {
	serverConn := player.connectedServer()
	if serverConn == nil {
		// No server connection yet, probably transitioning.
		return nil
	}
	serverMc := serverConn.conn()
	if serverMc != nil && serverConn.phase().ConsideredComplete() {
		return serverMc
	}
	return nil
}

func (c *clientPlaySessionHandler) Disconnected() {
	c.player.teardown()
}

// FlushQueuedPluginMessages immediately sends any queued messages to the server.
func (c *clientPlaySessionHandler) FlushQueuedPluginMessages() {
	serverConn := c.player.connectedServer()
	if serverConn == nil {
		return
	}
	serverMc, ok := serverConn.ensureConnected()
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.mu.loginPluginMessages.Len() != 0 {
		pm := c.mu.loginPluginMessages.PopFront()
		_ = serverMc.BufferPacket(pm)
	}
	_ = serverMc.Flush()
}

type (
	backendConnAdapter struct{ netmc.MinecraftConn }
	keepAliveAdapter   struct{ *connectedPlayer }
)

var (
	_ phase.BackendConn = (*backendConnAdapter)(nil)
	_ phase.KeepAlive   = (*keepAliveAdapter)(nil)
)

func (b *backendConnAdapter) FlushQueuedPluginMessages() {
	if h, ok := b.ActiveSessionHandler().(interface{ FlushQueuedPluginMessages() }); ok {
		h.FlushQueuedPluginMessages()
	}
}
func (k *keepAliveAdapter) SendKeepAlive() error {
	return netmc.SendKeepAlive(k)
}

func phaseHandle(
	player *connectedPlayer,
	backendConn netmc.MinecraftConn,
	msg *plugin.Message,
) bool {
	return player.phase().Handle(
		player,
		player,
		&keepAliveAdapter{player},
		&backendConnAdapter{backendConn},
		msg,
	)
}

func (c *clientPlaySessionHandler) handleKeepAlive(p *packet.KeepAlive) {
	forwardKeepAlive(p, c.player)
}

func forwardKeepAlive(p *packet.KeepAlive, player *connectedPlayer) {
	serverConn := player.connectedServer()
	if !sendKeepAliveToBackend(serverConn, player, p) {
		connInFlight := player.connectionInFlight()
		sendKeepAliveToBackend(connInFlight, player, p)
	}
}

func sendKeepAliveToBackend(serverConn *serverConnection, player *connectedPlayer, p *packet.KeepAlive) bool {
	if serverConn != nil {
		if sentTime, ok := serverConn.pendingPings.Get(p.RandomID); ok {
			serverConn.pendingPings.Delete(p.RandomID)
			if serverMc := serverConn.conn(); serverMc != nil {
				player.ping.Store(time.Since(sentTime))
				return serverMc.WritePacket(p) == nil
			}
		}
	}
	return false
}

func (c *clientPlaySessionHandler) handlePluginMessage(packet *plugin.Message) {
	serverConn := c.player.connectedServer()

	if serverConn == nil && packet.Channel == forge.LegacyHandshakeChannel {
		// Handling edge case when packet with FML client handshake (state COMPLETE)
		// arrives after JoinGame packet from destination server.
		serverConn = c.player.connectionInFlight()
	}

	var backendConn netmc.MinecraftConn
	if serverConn != nil {
		backendConn = serverConn.conn()
	}

	if serverConn == nil || backendConn == nil {
		return
	}

	if backendConn.State() != state.Play {
		c.log.Info("A plugin message was received while the backend server was not ready. Packet discarded.",
			"channel", packet.Channel)
	} else if plugin.IsRegister(packet) {
		if backendConn.WritePacket(packet) != nil {
			channelsIDs, _ := c.parseChannels(packet)
			c.proxy().event.Fire(&PlayerChannelRegisterEvent{
				channels: channelsIDs,
				player:   c.player,
			})
		}
	} else if plugin.IsUnregister(packet) {
		_ = backendConn.WritePacket(packet)
	} else if plugin.McBrand(packet) {
		brand := plugin.ReadBrandMessage(packet.Data)
		c.player.setClientBrand(brand)
		c.proxy().Event().FireParallel(&PlayerClientBrandEvent{
			player: c.player,
			brand:  brand,
		})
		_ = backendConn.WritePacket(plugin.RewriteMinecraftBrand(packet, c.player.Protocol()))
	} else {
		serverConnPhase := serverConn.phase()
		if serverConnPhase == phase.InTransitionBackendPhase {
			// We must bypass the currently-connected server when forwarding Forge packets.
			inFlight := c.player.connectionInFlight()
			if inFlight != nil {
				phaseHandle(c.player, inFlight.conn(), packet)
			}
			return
		}

		if phaseHandle(c.player, backendConn, packet) {
			return
		}
		if c.player.phase().ConsideredComplete() && serverConnPhase.ConsideredComplete() {
			id, ok := c.proxy().ChannelRegistrar().FromID(packet.Channel)
			if !ok {
				_ = backendConn.WritePacket(packet)
				return
			}
			clone := make([]byte, len(packet.Data))
			copy(clone, packet.Data)
			event.FireParallel(c.proxy().Event(), &PluginMessageEvent{
				source:     c.player,
				target:     serverConn,
				identifier: id,
				data:       clone,
				forward:    true,
			}, func(e *PluginMessageEvent) {
				if e.Allowed() {
					_ = backendConn.WritePacket(&plugin.Message{
						Channel: packet.Channel,
						Data:    clone,
					})
				}
			})
			return
		}
		// The client is trying to send messages too early. This is primarily caused by mods,
		// but further aggravated by Velocity. To work around these issues, we will queue any
		// non-FML handshake messages to be sent once the FML handshake has completed or the
		// JoinGame packet has been received by the proxy, whichever comes first.
		//
		// We also need to make sure to retain these packets, so they can be flushed
		// appropriately.
		c.mu.Lock()
		c.mu.loginPluginMessages.PushBack(packet)
		c.mu.Unlock()
	}
}

func (c *clientPlaySessionHandler) parseChannels(packet *plugin.Message) ([]message.ChannelIdentifier, []string) {
	var channels []string
	var channelsIDs []message.ChannelIdentifier
	channelIdentifiers := make(map[string]message.ChannelIdentifier)
	for _, channel := range plugin.Channels(packet) {
		id, err := message.ChannelIdentifierFrom(channel)
		if err != nil {
			c.log.V(1).Error(err, "got invalid channel in plugin message")
			continue
		}
		if _, ok := channelIdentifiers[id.ID()]; !ok { // deduplicate
			channelsIDs = append(channelsIDs, id)
			channels = append(channels, id.ID())
			channelIdentifiers[id.ID()] = id
		}
	}
	return channelsIDs, channels
}

// Handles the JoinGame packet and is responsible for handling the client-side
// switching servers in the proxy.
func (c *clientPlaySessionHandler) handleBackendJoinGame(pc *proto.PacketContext, joinGame *packet.JoinGame, destination *serverConnection) (err error) {
	serverMc, ok := destination.ensureConnected()
	if !ok {
		return errors.New("no backend server connection")
	}
	playerVersion := c.player.Protocol()
	if c.spawned.CompareAndSwap(false, true) {
		// The player wasn't spawned in yet, so we don't need to do anything special.
		// Just send JoinGame.
		if err = c.player.BufferPacket(joinGame); err != nil {
			return fmt.Errorf("error buffering %T for player: %w", joinGame, err)
		}
		// Required for Legacy Forge
		c.player.phase().OnFirstJoin(c.player)
	} else {
		// Clear tab list to avoid duplicate entries
		if err = c.player.tabList.RemoveAll(); err != nil {
			return fmt.Errorf("error clearing tablist entries: %w", err)
		}
		// The player is switching from a server already, so we need to tell the client to change
		// entity IDs and send new dimension information.
		if c.player.Type() == phase.LegacyForge {
			err = c.doSafeClientServerSwitch(joinGame)
			if err != nil {
				err = fmt.Errorf("error during safe client-server-switch: %w", err)
			}
		} else {
			err = c.doFastClientServerSwitch(joinGame, playerVersion)
			if err != nil {
				err = fmt.Errorf("error during fast client-server-switch: %w", err)
			}
		}
		if err != nil {
			return err
		}
	}

	// Remove previous boss bars. These don't get cleared when sending JoinGame, thus the need to
	// track them.
	c.mu.Lock()
	defer c.mu.Unlock()
	for barID := range c.mu.serverBossBars {
		deletePacket := &bossbar.BossBar{
			ID:     barID,
			Action: bossbar.RemoveAction,
		}
		if err = c.player.BufferPacket(deletePacket); err != nil {
			return fmt.Errorf("error buffering boss bar remove packet for player: %w", err)
		}
	}
	c.mu.serverBossBars = make(map[uuid.UUID]struct{}) // clear

	// Tell the server about the proxy's plugin message channels.
	serverVersion := serverMc.Protocol()
	channels := c.proxy().ChannelRegistrar().ChannelsForProtocol(serverVersion)
	if len(channels) != 0 {
		channelsPacket := plugin.ConstructChannelsPacket(serverVersion, channels.UnsortedList()...)
		if err = serverMc.BufferPacket(channelsPacket); err != nil {
			return fmt.Errorf("error buffering %T for backend: %w", channelsPacket, err)
		}
	}

	// If we had plugin messages queued during login/FML handshake, send them now.
	for c.mu.loginPluginMessages.Len() != 0 {
		pm := c.mu.loginPluginMessages.PopFront()
		if err = serverMc.BufferPacket(pm); err != nil {
			return fmt.Errorf("error buffering %T for backend: %w", pm, err)
		}
	}

	// Clear any title from the previous server.
	if playerVersion.GreaterEqual(version.Minecraft_1_8) {
		resetTitle, err := title.New(playerVersion, &title.Builder{Action: title.Reset})
		if err != nil {
			return err
		}
		if err = c.player.BufferPacket(resetTitle); err != nil {
			return fmt.Errorf("error buffering %T for player: %w", resetTitle, err)
		}
	}

	// Flush everything
	if err = c.player.Flush(); err != nil {
		return fmt.Errorf("error flushing buffered player packets: %w", err)
	}
	if serverMc.Flush() != nil {
		return fmt.Errorf("error flushing buffered backend packets: %w", err)
	}
	destination.completeJoin()
	return nil
}

func (c *clientPlaySessionHandler) doFastClientServerSwitch(joinGame *packet.JoinGame, playerVersion proto.Protocol) error {
	// In order to handle switching to another server, you will need to send two packets:
	//
	// - The join game packet from the backend server, with a different dimension
	// - A respawn with the correct dimension
	//
	// Most notably, by having the client accept the join game packet, we can work around the need
	// to perform entity ID rewrites, eliminating potential issues from rewriting packets and
	// improving compatibility with mods.
	respawn := respawnFromJoinGame(joinGame)

	// Since 1.16 this dynamic changed:
	// We don't need to send two dimension switches anymore!
	if playerVersion.Lower(version.Minecraft_1_16) {
		// Before Minecraft 1.16, we could not switch to the same dimension without sending an
		// additional respawn. On older versions of Minecraft this forces the client to perform
		// garbage collection which adds additional latency.
		if joinGame.Dimension == 0 {
			joinGame.Dimension = -1
		} else {
			joinGame.Dimension = 0
		}
	}
	var err error
	if err = c.player.BufferPacket(joinGame); err != nil {
		return fmt.Errorf("error buffering 1st %T for player: %w", joinGame, err)
	}

	if err = c.player.BufferPacket(respawn); err != nil {
		return fmt.Errorf("error buffering 2nd %T for player: %w", respawn, err)
	}
	return nil
}

func (c *clientPlaySessionHandler) doSafeClientServerSwitch(joinGame *packet.JoinGame) error {
	// Some clients do not behave well with the "fast" respawn sequence. In this case we will use
	// a "safe" respawn sequence that involves sending three packets to the client. They have the
	// same effect but tend to work better with buggier clients (Forge 1.8 in particular).

	var err error
	// Send the JoinGame packet itself, unmodified.
	if err = c.player.BufferPacket(joinGame); err != nil {
		return fmt.Errorf("error buffering 1st %T for player: %w", joinGame, err)
	}

	// Send a respawn packet in a different dimension.
	respawn := respawnFromJoinGame(joinGame)
	correctDim := respawn.Dimension
	if respawn.Dimension == 0 {
		respawn.Dimension = -1
	} else {
		respawn.Dimension = 0
	}
	if err = c.player.BufferPacket(respawn); err != nil {
		return fmt.Errorf("error buffering 2dn %T for player: %w", joinGame, err)
	}

	// Now send a respawn packet in the correct dimension.
	respawn.Dimension = correctDim
	if err = c.player.BufferPacket(respawn); err != nil {
		return fmt.Errorf("error buffering 3rd %T for player: %w", joinGame, err)
	}
	return nil
}

func respawnFromJoinGame(joinGame *packet.JoinGame) *packet.Respawn {
	return &packet.Respawn{
		Dimension:         joinGame.Dimension,
		PartialHashedSeed: joinGame.PartialHashedSeed,
		Difficulty:        joinGame.Difficulty,
		Gamemode:          joinGame.Gamemode,
		LevelType: func() string {
			if joinGame.LevelType != nil {
				return *joinGame.LevelType
			}
			return ""
		}(),
		DataToKeep:           0,
		DimensionInfo:        joinGame.DimensionInfo,
		PreviousGamemode:     joinGame.PreviousGamemode,
		CurrentDimensionData: joinGame.CurrentDimensionData,
		LastDeathPosition:    joinGame.LastDeathPosition,
		PortalCooldown:       joinGame.PortalCooldown,
		SeaLevel:             joinGame.SeaLevel,
	}
}

func (c *clientPlaySessionHandler) proxy() *Proxy {
	return c.player.proxy
}

func (c *clientPlaySessionHandler) handleLegacyChat(p *chat.LegacyChat) {
	_, ok := c.player.ensureBackendConnection()
	if !ok {
		return
	}
	if !c.validateChat(p.Message) {
		return
	}

	if strings.HasPrefix(p.Message, "/") {
		err := c.chatHandler.handleCommand(p)
		if err != nil {
			c.log.Error(err, "error handling legacy command")
		}
	} else {
		err := c.chatHandler.handleChat(p)
		if err != nil {
			c.log.Error(err, "error handling legacy chat")
		}
	}
}

func (c *clientPlaySessionHandler) handleSessionPlayerCommand(p *chat.SessionPlayerCommand) {
	_, ok := c.player.ensureBackendConnection()
	if !ok {
		return
	}
	if !c.updateTimeKeeper(p.Timestamp) {
		return
	}
	if !c.validateChat(p.Command) {
		return
	}

	err := c.chatHandler.handleCommand(p)
	if err != nil {
		c.log.Error(err, "error handling session player command")
	}
}

func (c *clientPlaySessionHandler) handleKeyedPlayerCommand(p *chat.KeyedPlayerCommand) {
	_, ok := c.player.ensureBackendConnection()
	if !ok {
		return
	}
	if !c.updateTimeKeeper(p.Timestamp) {
		return
	}
	if !c.validateChat(p.Command) {
		return
	}

	err := c.chatHandler.handleCommand(p)
	if err != nil {
		c.log.Error(err, "error handling keyed player command")
	}
}

func (c *clientPlaySessionHandler) handleSessionPlayerChat(p *chat.SessionPlayerChat) {
	_, ok := c.player.ensureBackendConnection()
	if !ok {
		return
	}
	if !c.updateTimeKeeper(p.Timestamp) {
		return
	}
	if !c.validateChat(p.Message) {
		return
	}

	err := c.chatHandler.handleChat(p)
	if err != nil {
		c.log.Error(err, "error handling session player chat")
	}
}

func (c *clientPlaySessionHandler) handleKeyedPlayerChat(p *chat.KeyedPlayerChat) {
	_, ok := c.player.ensureBackendConnection()
	if !ok {
		return
	}
	if !c.updateTimeKeeper(p.Expiry) {
		return
	}
	if !c.validateChat(p.Message) {
		return
	}

	err := c.chatHandler.handleChat(p)
	if err != nil {
		c.log.Error(err, "error handling keyed player chat")
	}
}

func (c *clientPlaySessionHandler) validateChat(msg string) bool {
	if validation.ContainsIllegalCharacter(msg) {
		c.player.Disconnect(illegalChatCharacters)
		return false
	}
	return true
}

func (c *clientPlaySessionHandler) handleTabCompleteRequest(p *packet.TabCompleteRequest, pc *proto.PacketContext) {
	isCommand := !p.AssumeCommand && strings.HasPrefix(p.Command, "/")
	if isCommand {
		c.handleCommandTabComplete(p, pc)
	} else {
		c.handleRegularTabComplete(p)
	}
}

func (c *clientPlaySessionHandler) handleCommandTabComplete(p *packet.TabCompleteRequest, pc *proto.PacketContext) {
	startPos := strings.LastIndex(p.Command, " ") + 1
	if !(startPos > 0) {
		return
	}

	// In 1.13+, we need to do additional work for the richer suggestions available.
	cmd := strings.TrimPrefix(p.Command, "/")
	cmdEndPosition := strings.Index(cmd, " ")
	if cmdEndPosition == -1 {
		cmdEndPosition = len(cmd)
	}

	commandLabel := cmd[:cmdEndPosition]
	if !c.proxy().command.Has(commandLabel) {
		if c.player.Protocol().Lower(version.Minecraft_1_13) {
			// Outstanding tab completes are recorded for use with 1.12 clients and below to provide
			// additional tab completion support.
			c.outstandingTabComplete = p
		}
		c.forwardToServer(pc)
		return
	}

	ctx, cancel := context.WithCancel(c.player.Context())
	defer cancel()
	suggestions, err := c.proxy().command.OfferSuggestions(ctx, c.player, cmd)
	if err != nil {
		c.log.Error(err, "Error while handling command tab completion for player",
			"command", cmd)
		return
	}
	if len(suggestions) == 0 {
		return
	}
	if c.log1.Enabled() {
		c.log1.Info("Response to TabCompleteRequest", "cmd", cmd, "suggestions", suggestions)
	}

	offers := make([]packet.TabCompleteOffer, 0, len(suggestions))
	for _, suggestion := range suggestions {
		offers = append(offers, packet.TabCompleteOffer{
			Text: suggestion,
			// TODO support brigadier tooltip
		})
	}

	// Send suggestions
	_ = c.player.WritePacket(&packet.TabCompleteResponse{
		TransactionID: p.TransactionID,
		Start:         startPos,
		Length:        len(p.Command) - startPos,
		Offers:        offers,
	})
}

func (c *clientPlaySessionHandler) handleRegularTabComplete(p *packet.TabCompleteRequest) {
	if c.player.Protocol().Lower(version.Minecraft_1_13) {
		// Outstanding tab completes are recorded for use with 1.12 clients and below to provide
		// additional tab completion support.
		c.outstandingTabComplete = p
	}
}

// handles additional tab complete
func (c *clientPlaySessionHandler) handleTabCompleteResponse(p *packet.TabCompleteResponse) {
	if c.outstandingTabComplete == nil || c.outstandingTabComplete.AssumeCommand {
		// Nothing to do
		_ = c.player.WritePacket(p)
		return
	}

	if strings.HasPrefix(c.outstandingTabComplete.Command, "/") {
		c.finishCommandTabComplete(c.outstandingTabComplete, p)
	} else {
		c.finishRegularTabComplete(c.outstandingTabComplete, p)
	}
	c.outstandingTabComplete = nil
}

func (c *clientPlaySessionHandler) finishCommandTabComplete(request *packet.TabCompleteRequest, response *packet.TabCompleteResponse) {
	cmd := request.Command[1:]
	offers, err := c.proxy().command.OfferSuggestions(context.Background(), c.player, cmd)
	if err != nil {
		c.log.Error(err, "Error while finishing command tab completion",
			"request", request, "response", response)
		return
	}
	legacy := c.player.Protocol().Lower(version.Minecraft_1_13)
	for _, offer := range offers {
		if legacy && !strings.HasPrefix(offer, "/") {
			offer = "/" + offer
		}
		if legacy && strings.HasPrefix(offer, cmd) {
			offer = offer[len(cmd):]
		}
		response.Offers = append(response.Offers, packet.TabCompleteOffer{
			Text: offer,
		})
	}
	// Sort offers alphabetically
	sort.Slice(response.Offers, func(i, j int) bool {
		return response.Offers[i].Text < response.Offers[j].Text
	})
	_ = c.player.WritePacket(response)
}

func (c *clientPlaySessionHandler) PlayerLog() logr.Logger {
	return c.player.log
}

func (c *clientPlaySessionHandler) finishRegularTabComplete(request *packet.TabCompleteRequest, response *packet.TabCompleteResponse) {
	offers := make([]string, 0, len(response.Offers))
	for _, offer := range response.Offers {
		offers = append(offers, offer.Text)
	}

	e := &TabCompleteEvent{
		player:         c.player,
		partialMessage: request.Command,
		suggestions:    offers,
	}
	c.proxy().event.Fire(e)
	response.Offers = nil
	for _, suggestion := range e.suggestions {
		response.Offers = append(response.Offers, packet.TabCompleteOffer{Text: suggestion})
	}
	_ = c.player.WritePacket(response)
}

type chatTimeKeeper struct{ lastTime time.Time }

func (k *chatTimeKeeper) update(t time.Time) bool {
	if t.Before(k.lastTime) {
		k.lastTime = t
		return false
	}
	k.lastTime = t
	return true
}

func (c *clientPlaySessionHandler) updateTimeKeeper(t time.Time) bool {
	if t.IsZero() {
		return true
	}
	if !c.chatTimeKeeper.update(t) {
		c.player.Disconnect(&component.Translation{
			Key: "multiplayer.disconnect.out_of_order_chat",
		})
		return false
	}
	return true
}

func (c *clientPlaySessionHandler) handleFinishedUpdate(p *config.FinishedUpdate) {
	// Complete client switch
	if !c.player.MinecraftConn.SwitchSessionHandler(state.Config) {
		panic("expected client to have config session handler")
	}
	serverConn := c.player.connectedServer()
	if serverConn != nil {
		smc, ok := serverConn.ensureConnected()
		if !ok {
			return
		}
		go func() {
			_ = smc.WritePacket(p)
			if !smc.SwitchSessionHandler(state.Config) {
				err := errors.New("failed to switch session handler")
				c.log.Error(err, "expected to switch session handler to config state")
				panic(err)
			}
			smc.SetAutoReading(true)
		}()
	}
	c.configSwitchFuture.Complete(nil)
}

func (c *clientPlaySessionHandler) handleChatAcknowledgement(p *chat.ChatAcknowledgement) {
	if c.player.CurrentServer() == nil {
		return
	}
	c.player.chatQueue.HandleAcknowledgement(p.Offset)
}

func (c *clientPlaySessionHandler) handleCookieResponse(p *cookie.CookieResponse) {
	// The payload in the cookie response packet has two bytes before the actual payload, which show the length.
	// So the packet should use the util.ReadBytesLen(rd, 5120) but unfortunately i couldn't get that to work, as it makes the payload empty.
	// TODO: fix this instead of this shortcut
	if len(p.Payload) > 2 {
		p.Payload = p.Payload[2:]
	}

	tracker := c.player.CookieRequestTracker
	requestID := fmt.Sprintf("%s:%s", c.player.ID(), p.Key)
	tracker.mu.Lock()
	responseChan, ok := tracker.pending[requestID]

	// check if the player.RequestCookieWithResult() is waiting for this packet, otherwise fire the event.
	if ok {
		responseChan <- p.Payload
	} else {
		event.FireParallel(c.player.eventMgr, newPlayerCookieResponseEvent(c.player, p.Key, p.Payload))
	}
	tracker.mu.Unlock()
}

// doSwitch handles switching stages for swapping between servers.
func (c *clientPlaySessionHandler) doSwitch() *future.Future[any] {
	c.log.V(1).WithName("doSwitch").Info("switching servers")

	existingConn := c.player.connectedServer()
	if existingConn != nil {
		// Shut down the existing server connection.
		c.player.setConnectedServer(nil)
		existingConn.disconnect()

		// Send keep alive to try to avoid timeouts
		if netmc.SendKeepAlive(c.player) != nil {
			return future.New[any]().Complete(nil)
		}

		// Config state clears everything in the client. No need to clear later.
		c.spawned.Store(false)
		c.mu.Lock()
		c.mu.serverBossBars = make(map[uuid.UUID]struct{}) // clear
		c.mu.Unlock()
		_ = c.player.tabList.RemoveAll()
		_ = tablist.ClearTabListHeaderFooter(c.player.tabList)
	}

	c.player.switchToConfigState()

	return &c.configSwitchFuture
}

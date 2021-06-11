package proxy

import (
	"context"
	"errors"
	"github.com/gammazero/deque"
	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/sets"
	"go.uber.org/atomic"
	"sort"
	"strings"
	"time"
)

// Handles communication with the connected Minecraft client.
// This is effectively the primary nerve center that joins backend servers with players.
type clientPlaySessionHandler struct {
	log, log1           logr.Logger
	player              *connectedPlayer
	spawned             atomic.Bool
	loginPluginMessages deque.Deque

	// TODO serverBossBars
	outstandingTabComplete *packet.TabCompleteRequest
}

func newClientPlaySessionHandler(player *connectedPlayer) *clientPlaySessionHandler {
	log := player.log.WithName("clientPlaySession")
	return &clientPlaySessionHandler{
		player: player,
		log:    log,
		log1:   log.V(1),
	}
}

func (c *clientPlaySessionHandler) handlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket {
		c.forwardToServer(pc)
		return
	}

	switch p := pc.Packet.(type) {
	case *packet.KeepAlive:
		c.handleKeepAlive(p)
	case *packet.Chat:
		c.handleChat(p)
	case *packet.TabCompleteRequest:
		c.handleTabCompleteRequest(p)
	case *plugin.Message:
		c.handlePluginMessage(p)
	case *packet.ClientSettings:
		c.player.setSettings(p)
		c.forwardToServer(pc) // forward to server
	default:
		c.forwardToServer(pc)
	}
}

func (c *clientPlaySessionHandler) deactivated() {
	c.loginPluginMessages.Clear()
}

func (c *clientPlaySessionHandler) activated() {
	protocol := c.player.Protocol()
	channels := c.player.proxy.ChannelRegistrar().ChannelsForProtocol(protocol)
	if len(channels) != 0 {
		register := plugin.ConstructChannelsPacket(protocol, channels.UnsortedList()...)
		_ = c.player.WritePacket(register)
		c.player.pluginChannelsMu.Lock()
		c.player.pluginChannels.InsertSet(channels)
		c.player.pluginChannelsMu.Unlock()
	}
}

func (c *clientPlaySessionHandler) forwardToServer(pc *proto.PacketContext) {
	if serverMc := c.canForward(); serverMc != nil {
		_ = serverMc.Write(pc.Payload)
	}
}

func (c *clientPlaySessionHandler) canForward() *minecraftConn {
	serverConn := c.player.connectedServer()
	if serverConn == nil {
		// No server connection yet, probably transitioning.
		return nil
	}
	serverMc := serverConn.conn()
	if serverMc != nil && serverConn.phase().consideredComplete() {
		return serverMc
	}
	return nil
}

func (c *clientPlaySessionHandler) disconnected() {
	c.player.teardown()
}

// Immediately send any queued messages to the server.
func (c *clientPlaySessionHandler) flushQueuedMessages() {
	serverConn := c.player.connectedServer()
	if serverConn == nil {
		return
	}
	serverMc, ok := serverConn.ensureConnected()
	if !ok {
		return
	}
	for c.loginPluginMessages.Len() != 0 {
		pm := c.loginPluginMessages.PopFront().(*plugin.Message)
		_ = serverMc.BufferPacket(pm)
	}
	_ = serverMc.flush()
}

func (c *clientPlaySessionHandler) handleKeepAlive(p *packet.KeepAlive) {
	serverConn := c.player.connectedServer()
	if serverConn != nil && p.RandomID == serverConn.lastPingID.Load() {
		serverMc := serverConn.conn()
		if serverMc != nil {
			lastPingSent := time.Unix(0, serverConn.lastPingSent.Load()*int64(time.Millisecond))
			c.player.ping.Store(time.Since(lastPingSent))
			if serverMc.WritePacket(p) == nil {
				serverConn.lastPingSent.Store(int64(time.Duration(time.Now().Nanosecond()) / time.Millisecond))
			}
		}
	}
}

func (c *clientPlaySessionHandler) handlePluginMessage(packet *plugin.Message) {
	serverConn := c.player.connectedServer()
	var backendConn *minecraftConn
	if serverConn != nil {
		backendConn = serverConn.conn()
	}

	if serverConn == nil || backendConn == nil {
		return
	}

	if backendConn.State() != state.Play {
		c.log.Info("A plugin message was received while the backend server was not ready. Packet discarded.",
			"channel", packet.Channel)
	} else if plugin.Register(packet) {
		if backendConn.WritePacket(packet) != nil {
			c.player.lockedKnownChannels(func(knownChannels sets.String) {
				knownChannels.Insert(plugin.Channels(packet)...)
			})
		}
	} else if plugin.Unregister(packet) {
		if backendConn.WritePacket(packet) != nil {
			c.player.lockedKnownChannels(func(knownChannels sets.String) {
				knownChannels.Delete(plugin.Channels(packet)...)
			})
		}
	} else if plugin.McBrand(packet) {
		_ = backendConn.WritePacket(plugin.RewriteMinecraftBrand(packet, c.player.Protocol()))
	} else {
		serverConnPhase := serverConn.phase()
		if serverConnPhase == inTransitionBackendPhase {
			// We must bypass the currently-connected server when forwarding Forge packets.
			inFlight := c.player.connectionInFlight()
			if inFlight != nil {
				c.player.phase().handle(inFlight, packet)
			}
			return
		}

		playerPhase := c.player.phase()
		if playerPhase.handle(serverConn, packet) {
			return
		}
		if playerPhase.consideredComplete() && serverConnPhase.consideredComplete() {
			id, ok := c.proxy().ChannelRegistrar().FromID(packet.Channel)
			if !ok {
				_ = backendConn.WritePacket(packet)
				return
			}
			clone := make([]byte, len(packet.Data))
			copy(clone, packet.Data)
			c.proxy().Event().FireParallel(&PluginMessageEvent{
				source:     c.player,
				target:     serverConn,
				identifier: id,
				data:       clone,
				forward:    true,
			}, func(ev event.Event) {
				e := ev.(*PluginMessageEvent)
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
		// We also need to make sure to retain these packets so they can be flushed
		// appropriately.
		c.loginPluginMessages.PushBack(packet)
	}
}

// Handles the JoinGame packet and is responsible for handling the client-side
// switching servers in the proxy.
func (c *clientPlaySessionHandler) handleBackendJoinGame(joinGame *packet.JoinGame, destination *serverConnection) (handled bool) {
	serverMc, ok := destination.ensureConnected()
	if !ok {
		return false
	}
	playerVersion := c.player.Protocol()
	if c.spawned.CAS(false, true) {
		// Nothing special to do with regards to spawning the player
		// Buffer JoinGame packet to player connection
		if c.player.BufferPacket(joinGame) != nil {
			return false
		}
		// Required for Legacy Forge
		c.player.phase().onFirstJoin(c.player)
	} else {
		// Clear tab list to avoid duplicate entries
		if c.player.tabList.clearEntries() != nil {
			return false
		}
		// TODO use instead: doFastClientServerSwitch, doSafeClientServerSwitch

		// In order to handle switching to another server, you will need to send two packets:
		//
		// - The join game packet from the backend server, with a different dimension
		// - A respawn with the correct dimension
		//
		// Most notably, by having the client accept the join game packet, we can work around the need
		// to perform entity ID rewrites, eliminating potential issues from rewriting packets and
		// improving compatibility with mods.
		if c.player.BufferPacket(joinGame) != nil {
			return false
		}
		respawn := &packet.Respawn{
			PartialHashedSeed: joinGame.PartialHashedSeed,
			Difficulty:        joinGame.Difficulty,
			Gamemode:          joinGame.Gamemode,
			LevelType: func() string {
				if joinGame.LevelType != nil {
					return *joinGame.LevelType
				}
				return ""
			}(),
			ShouldKeepPlayerData: false,
			DimensionInfo:        joinGame.DimensionInfo,
			PreviousGamemode:     joinGame.PreviousGamemode,
			CurrentDimensionData: joinGame.CurrentDimensionData,
		}

		// Since 1.16 this dynamic changed:
		// We don't need to send two dimension switches anymore!
		if playerVersion.Lower(version.Minecraft_1_16) {
			if joinGame.Dimension == 0 {
				respawn.Dimension = -1
			}
			if c.player.BufferPacket(respawn) != nil {
				return false
			}
		}

		respawn.Dimension = joinGame.Dimension
		if c.player.BufferPacket(respawn) != nil {
			return false
		}
	}

	// TODO Remove previous boss bars.
	// These don't get cleared when sending JoinGame, thus we need to track them.

	// Tell the server about this client's plugin message channels.
	serverVersion := serverMc.Protocol()
	playerKnownChannels := c.player.knownChannels().UnsortedList()
	if len(playerKnownChannels) != 0 {
		channelsPacket := plugin.ConstructChannelsPacket(serverVersion, playerKnownChannels...)
		if serverMc.BufferPacket(channelsPacket) != nil {
			return false
		}
	}

	// If we had plugin messages queued during login/FML handshake, send them now.
	for c.loginPluginMessages.Len() != 0 {
		pm := c.loginPluginMessages.PopFront().(*plugin.Message)
		if serverMc.BufferPacket(pm) != nil {
			return false
		}
	}

	// Clear any title from the previous server.
	if playerVersion.GreaterEqual(version.Minecraft_1_8) {
		resetTitle := packet.NewResetTitle(playerVersion)
		if c.player.BufferPacket(resetTitle) != nil {
			return false
		}
	}

	// Flush everything
	if c.player.flush() != nil || serverMc.flush() != nil {
		return false
	}
	destination.completeJoin()
	return true
}

var _ sessionHandler = (*clientPlaySessionHandler)(nil)

func (c *clientPlaySessionHandler) proxy() *Proxy {
	return c.player.proxy
}

func (c *clientPlaySessionHandler) handleChat(p *packet.Chat) {
	serverConn := c.player.connectedServer()
	if serverConn == nil {
		return
	}
	serverMc := serverConn.conn()
	if serverMc == nil {
		return
	}

	// Is it a command?
	if strings.HasPrefix(p.Message, "/") {
		commandline := trimSpaces(strings.TrimPrefix(p.Message, "/"))

		e := &CommandExecuteEvent{
			source:          c.player,
			commandline:     commandline,
			originalCommand: commandline,
		}
		c.proxy().event.Fire(e)
		forward, err := c.processCommandExecuteResult(e)
		if err != nil {
			c.log.Error(err, "Error while running command", "cmd", commandline)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return
		}
		if !forward {
			return
		}
	} else { // Is chat message
		e := &PlayerChatEvent{
			player:  c.player,
			message: p.Message,
		}
		c.proxy().Event().Fire(e)
		if !e.Allowed() || !c.player.Active() {
			return
		}
		// TODO
		c.log1.Info("Player sent chat message", "chat", p.Message)
	}

	// Forward message/command to server
	_ = serverMc.WritePacket(&packet.Chat{
		Message: p.Message,
		Type:    packet.ChatMessage,
		Sender:  c.player.ID(),
	})
}

func (c *clientPlaySessionHandler) processCommandExecuteResult(e *CommandExecuteEvent) (forward bool, err error) {
	if !e.Allowed() || !c.player.Active() {
		return false, nil
	}

	// Log player executed command
	log := c.log
	if e.Command() == e.OriginalCommand() {
		log = log.WithValues("command", e.Command())
	} else {
		log = log.WithValues("original", e.OriginalCommand(),
			"changed", e.Command())
	}
	log.Info("Player executed command")

	if !e.Forward() {
		hasRun, err := c.executeCommand(e.Command())
		if err != nil {
			return false, err
		}
		if hasRun {
			return false, nil // ran command, done
		}
	}

	// Forward command to server
	return true, nil
}

func (c *clientPlaySessionHandler) executeCommand(cmd string) (hasRun bool, err error) {
	// Make invoke context
	ctx, cancel := c.player.newContext(context.Background())
	defer cancel()

	// Dispatch command
	err = c.proxy().command.Do(ctx, c.player, cmd)
	if err != nil {
		if errors.Is(err, command.ErrForward) ||
			errors.Is(err, brigodier.ErrDispatcherUnknownCommand) {
			return false, nil // forward command to server
		}
		var sErr *brigodier.CommandSyntaxError
		if errors.As(err, &sErr) {
			return true, c.player.SendMessage(&component.Text{
				Content: sErr.Error(),
				S:       component.Style{Color: color.Red},
			})
		}
		return false, err
	}
	return true, nil
}

func (c *clientPlaySessionHandler) handleTabCompleteRequest(p *packet.TabCompleteRequest) {
	isCommand := !p.AssumeCommand && strings.HasPrefix(p.Command, "/")
	if isCommand {
		c.handleCommandTabComplete(p)
	} else {
		c.handleRegularTabComplete(p)
	}
}

func (c *clientPlaySessionHandler) handleCommandTabComplete(p *packet.TabCompleteRequest) {
	// In 1.13+, we need to do additional work for the richer suggestions available.
	cmd := p.Command[1:]
	cmdEndPosition := strings.Index(cmd, " ")
	if cmdEndPosition == -1 {
		cmdEndPosition = len(cmd)
	}

	commandLabel := cmd[:cmdEndPosition]
	if !c.proxy().command.Has(commandLabel) {
		if c.player.protocol.Lower(version.Minecraft_1_13) {
			// Outstanding tab completes are recorded for use with 1.12 clients and below to provide
			// additional tab completion support.
			c.outstandingTabComplete = p
		}
		return
	}

	ctx, cancel := c.player.newContext(context.Background())
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

	offers := make([]packet.TabCompleteOffer, 0, len(suggestions))
	for _, suggestion := range suggestions {
		offers = append(offers, packet.TabCompleteOffer{
			Text: suggestion,
		})
	}
	startPos := strings.Index(p.Command, " ") + 1
	if startPos > 0 {
		_ = c.player.WritePacket(&packet.TabCompleteResponse{
			TransactionID: p.TransactionID,
			Start:         startPos,
			Length:        len(p.Command) - startPos,
			Offers:        offers,
		})
	}
}

func (c *clientPlaySessionHandler) handleRegularTabComplete(p *packet.TabCompleteRequest) {
	if c.player.protocol.Lower(version.Minecraft_1_13) {
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
	legacy := c.player.protocol.Lower(version.Minecraft_1_13)
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

func (c *clientPlaySessionHandler) player_() *connectedPlayer {
	return c.player
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

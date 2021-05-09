package proxy

import (
	"context"
	"strings"
	"time"

	"github.com/gammazero/deque"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/sets"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/atomic"
)

// Handles communication with the connected Minecraft client.
// This is effectively the primary nerve center that joins backend servers with players.
type clientPlaySessionHandler struct {
	log                 logr.Logger
	player              *ConnectedPlayer
	spawned             atomic.Bool
	loginPluginMessages deque.Deque
	// serverBossBars
	// outstandingTabComplete TabCompleteRequest
}

func newClientPlaySessionHandler(player *ConnectedPlayer) *clientPlaySessionHandler {
	return &clientPlaySessionHandler{player: player, log: player.log.WithName("clientPlaySession")}
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
		if c.player.tabList.clearAll() != nil {
			return false
		}

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
	// These don't get cleared when sending JoinGame, thus the need to track them.

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
			source:      c.player,
			commandline: commandline,
		}
		c.proxy().event.Fire(e)
		if !e.Allowed() || !c.player.Active() {
			return
		}

		cmd, args, _ := extract(commandline)
		if c.proxy().command.Has(cmd) {
			// Make invoke context
			ctx, cancel := c.player.newContext(context.Background())
			defer cancel()
			// Invoke registered command
			c.log.Info("Player executing command", "cmd", commandline)
			_, err := c.proxy().command.Invoke(&Context{
				Context: ctx,
				Source:  c.player,
				Args:    args,
			}, cmd)
			if err != nil {
				c.log.Error(err, "Error invoking command", "cmd", commandline)
			}
			return
		}
		// Else, proxy command not registered, forward to server.
	} else {
		e := &PlayerChatEvent{
			player:  c.player,
			message: p.Message,
		}
		c.proxy().Event().Fire(e)
		if !e.Allowed() || !c.player.Active() {
			return
		}
		c.log.V(1).Info("Player sent chat message", "chat", p.Message)
	}

	// Forward to server
	_ = serverMc.WritePacket(&packet.Chat{
		Message: p.Message,
		Type:    packet.ChatMessage,
		Sender:  uuid.Nil,
	})
}

func (c *clientPlaySessionHandler) player_() *ConnectedPlayer {
	return c.player
}

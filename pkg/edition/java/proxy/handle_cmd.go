package proxy

import (
	"context"
	"errors"
	"go.minekube.com/gate/pkg/internal/future"
	"strings"
	"time"

	"go.minekube.com/brigodier"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/gate/proto"
)

func (c *chatHandler) handleCommand(packet proto.Packet) error {
	if c.player.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
		switch p := packet.(type) {
		case *chat.SessionPlayerCommand:
			return c.handleSessionCommand(p, false)
		case *chat.UnsignedPlayerCommand:
			return c.handleSessionCommand(&p.SessionPlayerCommand, true)
		}
	} else if c.player.Protocol().GreaterEqual(version.Minecraft_1_19) {
		if p, ok := packet.(*chat.KeyedPlayerCommand); ok {
			return c.handleKeyedCommand(p)
		}
	} else {
		if p, ok := packet.(*chat.LegacyChat); ok {
			return c.handleLegacyCommand(p)
		}
	}
	return nil
}

func (c *chatHandler) queueCommandResult(
	message string,
	timestamp time.Time,
	lastSeenMessages *chat.LastSeenMessages,
	packetCreator func(event *CommandExecuteEvent, lastSeenMessages *chat.LastSeenMessages) proto.Packet,
) {
	cmd := strings.TrimPrefix(message, "/")
	e := &CommandExecuteEvent{
		source:          c.player,
		commandline:     cmd,
		originalCommand: cmd,
	}
	c.eventMgr.Fire(e)

	c.player.chatQueue.QueuePacket(func(lastSeenMessages *chat.LastSeenMessages) *future.Future[proto.Packet] {
		f := future.New[proto.Packet]()
		go func() {
			pkt := packetCreator(e, lastSeenMessages)
			// TODO log command execution
			f.Complete(pkt)
		}()
		return f
	}, timestamp, lastSeenMessages)
}

func (c *chatHandler) handleLegacyCommand(packet *chat.LegacyChat) error {
	c.queueCommandResult(packet.Message, time.Now(), nil, func(e *CommandExecuteEvent, lastSeenMessages *chat.LastSeenMessages) proto.Packet {
		if !e.Allowed() {
			return nil
		}
		commandToRun := e.Command()
		if e.Forward() {
			return (&chat.Builder{
				Protocol: c.player.Protocol(),
				Message:  "/" + commandToRun,
				Sender:   c.player.ID(),
			}).ToServer()
		}

		hasRun, err := executeCommand(commandToRun, c.player, c.cmdMgr)
		if err != nil {
			c.log.Error(err, "error while running command", "command", commandToRun)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return nil
		}
		if !hasRun {
			return (&chat.Builder{
				Protocol: c.player.Protocol(),
				Message:  packet.Message,
				Sender:   c.player.ID(),
			}).ToServer()
		}
		return nil
	})
	return nil
}

func (c *chatHandler) handleKeyedCommand(packet *chat.KeyedPlayerCommand) error {
	c.queueCommandResult(packet.Command, packet.Timestamp, nil, func(e *CommandExecuteEvent, lastSeenMessages *chat.LastSeenMessages) proto.Packet {

		playerKey := c.player.IdentifiedKey()
		if !e.Allowed() {
			if playerKey != nil {
				if !packet.Unsigned && keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2) {
					if c.disconnectIllegalProtocolState(c.player) {
						c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
					}
				}
			}
			return nil
		}

		commandToRun := e.Command()
		if e.Forward() {
			if !packet.Unsigned && commandToRun == packet.Command {
				return packet
			} else {
				if !packet.Unsigned && playerKey != nil && keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2) {
					if c.disconnectIllegalProtocolState(c.player) {
						c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
					}
					return nil
				}
				return (&chat.Builder{
					Protocol:  c.player.Protocol(),
					Message:   "/" + commandToRun,
					Sender:    c.player.ID(),
					Timestamp: packet.Timestamp,
				}).ToServer()
			}
		}

		hasRun, err := executeCommand(commandToRun, c.player, c.cmdMgr)
		if err != nil {
			c.log.Error(err, "error while running command", "command", commandToRun)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return nil
		}
		if !hasRun {
			if commandToRun == packet.Command {
				return packet
			}

			if !packet.Unsigned && playerKey != nil &&
				keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2) &&
				c.disconnectIllegalProtocolState(c.player) {
				c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
				return nil
			}

			return (&chat.Builder{
				Protocol:  c.player.Protocol(),
				Message:   "/" + commandToRun,
				Sender:    c.player.ID(),
				Timestamp: packet.Timestamp,
			}).ToServer()
		}
		return nil
	})

	return nil
}

func (c *chatHandler) handleSessionCommand(packet *chat.SessionPlayerCommand, unsigned bool) error {
	consumeCommand := func(packet *chat.SessionPlayerCommand, hasLastSeenMessages bool) proto.Packet {
		if !hasLastSeenMessages {
			return nil
		}
		if packet.Signed() {
			if c.disconnectIllegalProtocolState(c.player) {
				c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
			}
			return nil
		}

		// An unsigned command with a 'last seen' update will not happen as of 1.20.5+, but for earlier versions - we still
		// need to pass through the acknowledgement
		if c.player.Protocol().GreaterEqual(version.Minecraft_1_19_3) && !packet.LastSeenMessages.Empty() {
			return &chat.ChatAcknowledgement{
				Offset: packet.LastSeenMessages.Offset,
			}
		}
		return nil
	}

	modifyCommand := func(packet *chat.SessionPlayerCommand, newCommand string) proto.Packet {
		if packet.Signed() && c.disconnectIllegalProtocolState(c.player) {
			c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
			return nil
		}
		return (&chat.Builder{
			Protocol:  c.player.Protocol(),
			Message:   "/" + newCommand,
			Sender:    c.player.ID(),
			Timestamp: packet.Timestamp,
		}).ToServer()
	}

	forwardCommand := func(packet *chat.SessionPlayerCommand, newCommand string) proto.Packet {
		if newCommand == packet.Command {
			return packet
		}
		return modifyCommand(packet, newCommand)
	}

	c.queueCommandResult(packet.Command, packet.Timestamp, &packet.LastSeenMessages, func(e *CommandExecuteEvent, newLastSeenMessages *chat.LastSeenMessages) proto.Packet {
		if newLastSeenMessages != nil && !unsigned {
			packet.LastSeenMessages = *newLastSeenMessages // fixed packet
		}

		if !e.Allowed() {
			return consumeCommand(packet, newLastSeenMessages != nil)
		}

		commandToRun := e.Command()
		if e.Forward() {
			return forwardCommand(packet, commandToRun)
		}

		hasRun, err := executeCommand(commandToRun, c.player, c.cmdMgr)
		if err != nil {
			c.log.Error(err, "error while running command", "command", commandToRun)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return nil
		}
		if hasRun {
			return consumeCommand(packet, newLastSeenMessages != nil)
		}
		return forwardCommand(packet, commandToRun)
	})
	return nil
}

func executeCommand(cmd string, player *connectedPlayer, cmdMgr *command.Manager) (hasRun bool, err error) {
	// Make invoke context
	ctx, cancel := context.WithCancel(player.Context())
	defer cancel()

	// Dispatch command
	err = cmdMgr.Do(ctx, player, cmd)
	if err != nil {
		// TODO add event to handle for unknown command and command with syntax error
		if errors.Is(err, command.ErrForward) ||
			errors.Is(err, brigodier.ErrDispatcherUnknownCommand) {
			return false, nil // forward command to server
		}
		var sErr *brigodier.CommandSyntaxError
		if errors.As(err, &sErr) {
			return true, player.SendMessage(&component.Text{
				Content: sErr.Error(),
				S:       component.Style{Color: color.Red},
			})
		}
		return false, err
	}
	return true, nil
}

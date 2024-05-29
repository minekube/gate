package proxy

import (
	"context"
	"errors"
	"strings"

	"github.com/robinbraemer/event"
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
			return c.handleSessionCommand(p)
		case *chat.UnsignedPlayerCommand:
			return c.handleSessionCommand(&p.SessionPlayerCommand)
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

func (c *chatHandler) handleLegacyCommand(packet *chat.LegacyChat) error {
	cmd := strings.TrimPrefix(packet.Message, "/")
	e := &CommandExecuteEvent{
		source:          c.player,
		commandline:     cmd,
		originalCommand: cmd,
	}
	event.FireParallel(c.eventMgr, e, func(e *CommandExecuteEvent) {
		if !e.Allowed() {
			return
		}
		server, ok := c.player.ensureBackendConnection()
		if !ok {
			return
		}
		commandToRun := e.Command()
		if e.Forward() {
			_ = server.WritePacket((&chat.Builder{
				Protocol: server.Protocol(),
				Message:  "/" + commandToRun,
				Sender:   c.player.ID(),
			}).ToServer())
			return
		}

		hasRun, err := executeCommand(commandToRun, c.player, c.cmdMgr)
		if err != nil {
			c.log.Error(err, "error while running command", "command", commandToRun)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return
		}
		if !hasRun {
			_ = server.WritePacket((&chat.Builder{
				Protocol: server.Protocol(),
				Message:  packet.Message,
				Sender:   c.player.ID(),
			}).ToServer())
		}
	})
	return nil
}

func (c *chatHandler) handleKeyedCommand(packet *chat.KeyedPlayerCommand) error {
	e := &CommandExecuteEvent{
		source:          c.player,
		commandline:     packet.Command,
		originalCommand: packet.Command,
	}
	event.FireParallel(c.eventMgr, e, func(e *CommandExecuteEvent) {
		playerKey := c.player.IdentifiedKey()
		if !e.Allowed() {
			if playerKey != nil {
				if !packet.Unsigned && keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2) {
					if c.disconnectIllegalProtocolState(c.player) {
						c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
					}
					return
				}
			}
			return
		}

		commandToRun := e.Command()
		if e.Forward() {
			server, ok := c.player.ensureBackendConnection()
			if !ok {
				return
			}
			if !packet.Unsigned && commandToRun == packet.Command {
				_ = server.WritePacket(packet)
				return
			}
			if !packet.Unsigned && playerKey != nil && keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2) {
				if c.disconnectIllegalProtocolState(c.player) {
					c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
				}
				return
			}
			_ = server.WritePacket((&chat.Builder{
				Protocol:  server.Protocol(),
				Message:   packet.Command,
				Sender:    c.player.ID(),
				Timestamp: packet.Timestamp,
			}).ToServer())
			return
		}

		hasRun, err := executeCommand(commandToRun, c.player, c.cmdMgr)
		if err != nil {
			c.log.Error(err, "error while running command", "command", commandToRun)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return
		}
		if !hasRun {
			server, ok := c.player.ensureBackendConnection()
			if !ok {
				return
			}
			if commandToRun == packet.Command {
				_ = server.WritePacket(packet)
				return
			}

			if !packet.Unsigned && playerKey != nil &&
				keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2) &&
				c.disconnectIllegalProtocolState(c.player) {
				c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
				return
			}

			_ = server.WritePacket((&chat.Builder{
				Protocol:  server.Protocol(),
				Message:   "/" + commandToRun,
				Sender:    c.player.ID(),
				Timestamp: packet.Timestamp,
			}).ToServer())
		}
	})
	return nil
}

func (c *chatHandler) handleSessionCommand(packet *chat.SessionPlayerCommand) error {
	e := &CommandExecuteEvent{
		source:          c.player,
		commandline:     packet.Command,
		originalCommand: packet.Command,
	}
	event.FireParallel(c.eventMgr, e, func(e *CommandExecuteEvent) {
		server, ok := c.player.ensureBackendConnection()
		if !ok {
			return
		}
		if !e.Allowed() {
			if packet.Signed() {
				if c.disconnectIllegalProtocolState(c.player) {
					c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
				}
				return
			}
			// We seemingly can't actually do this if signed args exist, if not, we can probs keep stuff happy
			if c.player.Protocol().GreaterEqual(version.Minecraft_1_19_3) && !packet.LastSeenMessages.Empty() {
				_ = server.WritePacket(&chat.ChatAcknowledgement{
					Offset: packet.LastSeenMessages.Offset,
				})
			}
			return
		}

		commandToRun := e.Command()
		if e.Forward() {
			if packet.Signed() && commandToRun == packet.Command {
				_ = server.WritePacket(packet)
				return
			}
			if packet.Signed() && c.disconnectIllegalProtocolState(c.player) {
				c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
				return
			}
			_ = server.WritePacket((&chat.Builder{
				Protocol:  server.Protocol(),
				Message:   "/" + commandToRun,
				Sender:    c.player.ID(),
				Timestamp: packet.Timestamp,
			}).ToServer())
			return
		}

		hasRun, err := executeCommand(commandToRun, c.player, c.cmdMgr)
		if err != nil {
			c.log.Error(err, "error while running command", "command", commandToRun)
			_ = c.player.SendMessage(&component.Text{
				Content: "An error occurred while running this command.",
				S:       component.Style{Color: color.Red},
			})
			return
		}
		if !hasRun {
			server, ok := c.player.ensureBackendConnection()
			if !ok {
				return
			}
			if packet.Signed() && commandToRun == packet.Command {
				_ = server.WritePacket(packet)
				return
			}
			if packet.Signed() && c.disconnectIllegalProtocolState(c.player) {
				c.log.Info("A plugin tried to deny a command with signable component(s). This is not supported with forceKeyAuthentication enabled.")
				return
			}
			_ = server.WritePacket((&chat.Builder{
				Protocol:  server.Protocol(),
				Message:   "/" + commandToRun,
				Sender:    c.player.ID(),
				Timestamp: packet.Timestamp,
			}).ToServer())
		}
		if c.player.Protocol().GreaterEqual(version.Minecraft_1_19_3) && !packet.LastSeenMessages.Empty() {
			_ = server.WritePacket(&chat.ChatAcknowledgement{
				Offset: packet.LastSeenMessages.Offset,
			})
		}
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

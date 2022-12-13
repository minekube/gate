package proxy

import (
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/gate/proto"
)

type chatHandler struct {
	log      logr.Logger
	eventMgr event.Manager
	player   *connectedPlayer
	cmdMgr   *command.Manager
}

func (c *chatHandler) handleChat(packet proto.Packet) error {
	if c.player.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
		if p, ok := packet.(*chat.SessionPlayerChat); ok {
			return c.handleSessionChat(p)
		}
	} else if c.player.Protocol().GreaterEqual(version.Minecraft_1_19) {
		if p, ok := packet.(*chat.KeyedPlayerChat); ok {
			return c.handleKeyedChat(p)
		}
	} else {
		if p, ok := packet.(*chat.LegacyChat); ok {
			return c.handleLegacyChat(p)
		}
	}
	return nil
}

func (c *chatHandler) handleLegacyChat(packet *chat.LegacyChat) error {
	server, ok := c.player.ensureBackendConnection()
	if !ok {
		return nil
	}
	evt := &PlayerChatEvent{
		player:   c.player,
		original: packet.Message,
	}
	c.eventMgr.Fire(evt)
	if !evt.Allowed() {
		return nil
	}
	return server.WritePacket((&chat.Builder{
		Protocol: server.Protocol(),
		Message:  evt.Message(),
		Sender:   evt.player.ID(),
	}).ToServer())
}

//type ChatQueue interface {
//	// Enqueue enqueues a chat message to be sent to the server.
//	// The message is sent to the server when the player is connected to the server.
//	Enqueue(message string)
//}

func (c *chatHandler) handleSessionChat(packet *chat.SessionPlayerChat) error {
	server, ok := c.player.ensureBackendConnection()
	if !ok {
		return nil
	}
	evt := &PlayerChatEvent{
		player:   c.player,
		original: packet.Message,
	}
	c.eventMgr.Fire(evt)
	if !evt.Allowed() {
		if packet.Signed {
			invalidCancel(c.log, c.player)
		}
		return nil
	}

	if evt.Message() != evt.Original() {
		if packet.Signed {
			invalidChange(c.log, c.player)
			return nil
		}
		return server.WritePacket((&chat.Builder{
			Protocol:  server.Protocol(),
			Message:   packet.Message,
			Sender:    c.player.ID(),
			Timestamp: packet.Timestamp,
		}).ToServer())
	}
	return server.WritePacket(packet)
}

func (c *chatHandler) handleKeyedChat(packet *chat.KeyedPlayerChat) error {
	server, ok := c.player.ensureBackendConnection()
	if !ok {
		return nil
	}
	evt := &PlayerChatEvent{
		player:   c.player,
		original: packet.Message,
	}
	c.eventMgr.Fire(evt)

	if c.player.IdentifiedKey() != nil && !packet.Unsigned {
		// 1.19->1.19.2 signed version
		return c.handleOldSignedChat(server, packet, evt)
	}
	// 1.19->1.19.2 unsigned version
	if !evt.Allowed() {
		return nil
	}
	return server.WritePacket((&chat.Builder{
		Protocol:  server.Protocol(),
		Message:   evt.Message(),
		Sender:    c.player.ID(),
		Timestamp: packet.Expiry,
	}).ToServer())
}

func (c *chatHandler) handleOldSignedChat(server netmc.MinecraftConn, packet *chat.KeyedPlayerChat, event *PlayerChatEvent) error {
	playerKey := c.player.IdentifiedKey()
	denyRevision := keyrevision.RevisionIndex(playerKey.KeyRevision()) >= keyrevision.RevisionIndex(keyrevision.LinkedV2)
	if !event.Allowed() && denyRevision {
		invalidCancel(c.log, c.player)
		return nil
	}

	if event.Message() != packet.Message {
		if denyRevision {
			// Bad, very bad.
			invalidChange(c.log, c.player)
			return nil
		}
		c.log.Info("a plugin changed a signed chat message. The server may not accept it")
		return server.WritePacket((&chat.Builder{
			Protocol:  server.Protocol(),
			Message:   event.Message(),
			Sender:    c.player.ID(),
			Timestamp: packet.Expiry,
		}).ToServer())
	}
	return server.WritePacket(packet)
}

func invalidCancel(log logr.Logger, player *connectedPlayer) {
	invalidMessage(log.WithName("invalidCancel"), player)
}

func invalidChange(log logr.Logger, player *connectedPlayer) {
	invalidMessage(log.WithName("invalidChange"), player)
}

func invalidMessage(log logr.Logger, player *connectedPlayer) {
	log.Info("A plugin tried to cancel a signed chat message." +
		" This is no longer possible in 1.19.1 and newer. " +
		"Disconnecting player...")
	disconnectIllegalProtocolState(player)
}

func disconnectIllegalProtocolState(player *connectedPlayer) {
	player.Disconnect(&component.Text{
		Content: "A proxy plugin caused an illegal protocol state. Contact your network administrator.",
		S:       component.Style{Color: color.Red},
	})
}

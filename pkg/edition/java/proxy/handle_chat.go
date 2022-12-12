package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

func handleChat(protocol proto.Protocol) {
	if protocol.GreaterEqual(version.Minecraft_1_19_3) {
		handleSessionChat()
	} else if protocol.GreaterEqual(version.Minecraft_1_19) {
		handleKeyedChat()
	} else {
		handleLegacyChat()
	}
}

func handleSessionChat() {

}

func handleKeyedChat() {

}

type chatHandler[T proto.Packet] interface {
	handleChat(T)
}

type legacyChatHandler struct {
	*clientPlaySessionHandler
}

func (c *legacyChatHandler) handle(packet *chat.LegacyChat) error {
	server, ok := c.player.ensureBackendConnection()
	if !ok {
		return nil
	}
	event := &PlayerChatEvent{
		player:   c.player,
		original: packet.Message,
	}
	c.proxy().event.Fire(event)
	if !event.Allowed() {
		return nil
	}
	return server.WritePacket((&chat.Builder{
		Message: event.Message(),
		Sender:  event.player.ID(),
	}).ToServer())
}

type sessionChatHandler struct {
	*clientPlaySessionHandler
}

type ChatQueue interface {
	// Enqueue enqueues a chat message to be sent to the server.
	// The message is sent to the server when the player is connected to the server.
	Enqueue(message string)
}

func (c *sessionChatHandler) handle(packet *chat.SessionPlayerChat) error {
	server, ok := c.player.ensureBackendConnection()
	if !ok {
		return nil
	}
	return nil
}

package chat

import (
	"strings"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	MaxServerBoundMessageLength = 256
)

// MessageType is the position a chat message is going to be sent.
type MessageType byte

const (
	// ChatMessageType lets the chat message appear in the client's HUD.
	// These messages can be filtered out by the client's settings.
	ChatMessageType MessageType = iota
	// SystemMessageType lets the chat message appear in the client's HUD and can't be dismissed.
	SystemMessageType
	// GameInfoMessageType lets the chat message appear above the player's main HUD.
	// This text format doesn't support many component features, such as hover events.
	GameInfoMessageType
)

// Builder is a builder for protocol aware chat messages.
type Builder struct {
	// Protocol is the protocol version of the message.
	// If not set, defaults to supporting older clients.
	Protocol proto.Protocol
	// Component is the component to send to the client.
	// If nil, Message is used instead.
	Component component.Component
	Message   string // Only used if Component is nil
	// Type is the position the message is going to be sent.
	// If not set, defaults to ChatMessageType.
	Type MessageType
	// Sender is the UUID of the player who sent the message.
	// If zero (uuid.Nil), the message is sent by the server.
	Sender uuid.UUID
	// Timestamp is the time the message was sent.
	Timestamp time.Time
}

// ToClient creates a packet which can be sent to the client;
// using the provided information in the builder.
func (b *Builder) ToClient() proto.Packet {
	msg := b.Component
	if msg == nil {
		msg = &component.Text{Content: b.Message}
	}

	if b.Protocol.GreaterEqual(version.Minecraft_1_19) {
		t := b.Type
		if t == ChatMessageType {
			t = SystemMessageType
		}
		return &SystemChat{
			Component: &ComponentHolder{
				Protocol:  b.Protocol,
				Component: msg,
			},
			Type: t,
		}
	}

	buf := new(strings.Builder)
	_ = util.JsonCodec(b.Protocol).Marshal(buf, msg)
	return &LegacyChat{
		Message: buf.String(),
		Type:    b.Type,
		Sender:  b.Sender,
	}
}

// ToServer creates a packet which can be sent to the server;
// using the provided information in the builder.
func (b *Builder) ToServer() proto.Packet {
	if b.Timestamp.IsZero() {
		b.Timestamp = time.Now()
	}
	if b.Protocol.GreaterEqual(version.Minecraft_1_19_3) { // Session chat
		if strings.HasPrefix(b.Message, "/") {
			if b.Protocol.GreaterEqual(version.Minecraft_1_20_5) {
				return &UnsignedPlayerCommand{
					SessionPlayerCommand: SessionPlayerCommand{
						Command: strings.TrimPrefix(b.Message, "/"),
					},
				}
			}
			return &SessionPlayerCommand{
				Command:   strings.TrimPrefix(b.Message, "/"),
				Timestamp: b.Timestamp,
			}
		}
		return &SessionPlayerChat{
			Message:   b.Message,
			Timestamp: b.Timestamp,
			Signature: []byte{0},
		}
	} else if b.Protocol.GreaterEqual(version.Minecraft_1_19) { // Keyed chat
		if strings.HasPrefix(b.Message, "/") {
			return NewKeyedPlayerCommand(strings.TrimPrefix(b.Message, "/"), nil, b.Timestamp)
		}
		// This will produce an error on the server, but needs to be here.
		return &KeyedPlayerChat{
			Message:  b.Message,
			Unsigned: true,
			Expiry:   b.Timestamp,
		}
	}
	// Legacy chat
	return &LegacyChat{Message: b.Message}
}

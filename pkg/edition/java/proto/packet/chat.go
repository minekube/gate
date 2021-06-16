package packet

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
)

const (
	MaxServerBoundMessageLength = 256
)

type Chat struct {
	Message string
	Type    MessageType
	Sender  uuid.UUID // 1.16+, and can be empty UUID, all zeros
}

func (ch *Chat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, ch.Message)
	if err != nil {
		return err
	}
	if c.Direction == proto.ClientBound && c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		err = util.WriteByte(wr, byte(ch.Type))
		if err != nil {
			return err
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
			err = util.WriteUUID(wr, ch.Sender)
		}
	}
	return err
}

func (ch *Chat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	ch.Message, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	if c.Direction == proto.ClientBound && c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		var pos byte
		pos, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
		ch.Type = MessageType(pos)
		if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
			ch.Sender, err = util.ReadUUID(rd)
			if err != nil {
				return err
			}
		}
	}
	return
}

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

var _ proto.Packet = (*Chat)(nil)

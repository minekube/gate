package packet

import (
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/util"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
)

const (
	MaxServerBoundMessageLength = 256
)

type Chat struct {
	Message string
	Type    MessagePosition
	Sender  uuid.UUID // 1.16+, and can empty UUID, all zeros
}

func (ch *Chat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, ch.Message)
	if err != nil {
		return err
	}
	if c.Direction == proto.ClientBound && c.Protocol.GreaterEqual(proto.Minecraft_1_8) {
		err = util.WriteByte(wr, byte(ch.Type))
		if err != nil {
			return err
		}
		if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
			err = util.WriteUuid(wr, ch.Sender)
		}
	}
	return err
}

func (ch *Chat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	ch.Message, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	if c.Direction == proto.ClientBound && c.Protocol.GreaterEqual(proto.Minecraft_1_8) {
		var pos byte
		pos, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
		ch.Type = MessagePosition(pos)
		if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
			ch.Sender, err = util.ReadUUID(rd)
			if err != nil {
				return err
			}
		}
	}
	return
}

// MessagePosition is the position a chat message is going to be sent.
type MessagePosition byte

const (
	// The chat message will appear in the client's HUD.
	// These messages can be filtered out by the client.
	ChatMessage MessagePosition = iota
	// The chat message will appear in the client's HUD and can't be dismissed.
	SystemMessage
	// The chat message will appear above the player's main HUD.
	// This text format doesn't support many component features, such as hover events.
	ActionBarMessage
)

var _ proto.Packet = (*Chat)(nil)

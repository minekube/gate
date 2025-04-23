package chat

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type LegacyChat struct {
	Message string
	Type    MessageType
	Sender  uuid.UUID // 1.16+, and can be empty UUID, all zeros
}

var _ proto.Packet = (*LegacyChat)(nil)

func (ch *LegacyChat) Encode(c *proto.PacketContext, wr io.Writer) error {
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

func (ch *LegacyChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	max := 100
	if c.Direction == proto.ClientBound {
		max = 262144
	} else if c.Protocol.GreaterEqual(version.Minecraft_1_11) {
		max = 256
	}
	
	ch.Message, err = util.ReadStringMax(rd, max)
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

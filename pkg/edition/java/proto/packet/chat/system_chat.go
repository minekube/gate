package chat

import (
	"fmt"
	"io"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type SystemChat struct {
	Component component.Component
	Type      MessageType
}

func (p *SystemChat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteComponent(wr, c.Protocol, p.Component)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		switch p.Type {
		case SystemMessageType:
			err = util.WriteBool(wr, false)
		case GameInfoMessageType:
			err = util.WriteBool(wr, true)
		default:
			return fmt.Errorf("invalid chat type: %d", p.Type)
		}
		return err
	}
	return util.WriteVarInt(wr, int(p.Type))
}

func (p *SystemChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.Component, err = util.ReadComponent(rd, c.Protocol)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		typ, err := util.ReadBool(rd)
		if err != nil {
			return err
		}
		p.Type = SystemMessageType
		if typ {
			p.Type = GameInfoMessageType
		}
		return nil
	}
	typ, err := util.ReadVarInt(rd)
	p.Type = MessageType(typ)
	return
}

var _ proto.Packet = (*SystemChat)(nil)

package packet

import (
	"io"

	"github.com/Tnze/go-mc/nbt"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

type DialogClear struct{}

var _ proto.Packet = (*DialogClear)(nil)

func (d *DialogClear) Encode(c *proto.PacketContext, wr io.Writer) error {
	return nil
}

func (d *DialogClear) Decode(c *proto.PacketContext, rd io.Reader) error {
	return nil
}

type DialogShow struct {
	State     states.State
	ID        int
	BinaryTag nbt.RawMessage
}

var _ proto.Packet = (*DialogShow)(nil)

func (d *DialogShow) Encode(c *proto.PacketContext, wr io.Writer) error {
	if d.State == states.ConfigState {
		return util.WriteBinaryTag(wr, c.Protocol, d.BinaryTag)
	}
	util.PWriteVarInt(wr, d.ID)
	if d.ID == 0 {
		return util.WriteBinaryTag(wr, c.Protocol, d.BinaryTag)
	}
	return nil
}

func (d *DialogShow) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if d.State == states.ConfigState {
		d.ID = 0
	} else {
		d.ID, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	if d.ID == 0 {
		d.BinaryTag, err = util.ReadBinaryTag(rd, c.Protocol)
		return err
	}
	return nil
}

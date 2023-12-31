package chat

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type ChatAcknowledgement struct {
	Offset int
}

var _ proto.Packet = (*ChatAcknowledgement)(nil)

func (c *ChatAcknowledgement) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteVarInt(wr, c.Offset)
}

func (c *ChatAcknowledgement) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	c.Offset, err = util.ReadVarInt(rd)
	return
}

package packet

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type PingIdentify struct {
	ID int
}

var _ proto.Packet = (*PingIdentify)(nil)

func (p *PingIdentify) Decode(c *proto.PacketContext, rd io.Reader) error {
	var err error
	p.ID, err = util.ReadInt(rd)
	return err
}

func (p *PingIdentify) Encode(c *proto.PacketContext, wr io.Writer) error {
	return util.WriteInt(wr, p.ID)
}

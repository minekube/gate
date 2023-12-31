package config

import (
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type StartUpdate struct{}

var _ proto.Packet = (*StartUpdate)(nil)

func (p *StartUpdate) Encode(c *proto.PacketContext, wr io.Writer) error {
	return nil
}

func (p *StartUpdate) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	return nil
}

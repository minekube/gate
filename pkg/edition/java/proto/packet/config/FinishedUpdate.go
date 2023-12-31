package config

import (
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type FinishedUpdate struct{}

var _ proto.Packet = (*FinishedUpdate)(nil)

func (p *FinishedUpdate) Encode(c *proto.PacketContext, wr io.Writer) error {
	return nil
}

func (p *FinishedUpdate) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	return nil
}

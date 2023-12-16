package config

import (
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type RegistrySync struct {
	Data []byte
}

var _ proto.Packet = (*RegistrySync)(nil)

func (p *RegistrySync) Encode(c *proto.PacketContext, wr io.Writer) error {
	_, err := wr.Write(p.Data)
	return err
}

func (p *RegistrySync) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	// NBT change in 1.20.2 makes it difficult to parse this packet.
	p.Data, err = io.ReadAll(rd)
	return err
}

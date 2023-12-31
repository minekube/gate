package config

import (
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type ActiveFeatures struct {
	ActiveFeatures []key.Key
}

var _ proto.Packet = (*ActiveFeatures)(nil)

func (p *ActiveFeatures) Encode(c *proto.PacketContext, wr io.Writer) error {
	return util.WriteKeyArray(wr, p.ActiveFeatures)
}

func (p *ActiveFeatures) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.ActiveFeatures, err = util.ReadKeyArray(rd)
	return err
}

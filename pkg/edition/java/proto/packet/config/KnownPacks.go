package config

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
	"io"
)

const MaxLengthPacks = 64

// ErrTooManyPacks is returned when sends too many packs.
var ErrTooManyPacks = errs.NewSilentErr("too many packs")

type KnownPacks struct {
	Packs []KnownPack
}

func (p *KnownPacks) Decode(c *proto.PacketContext, rd io.Reader) error {
	packCount := util.PReadIntVal(rd)
	if packCount > MaxLengthPacks {
		return ErrTooManyPacks
	}
	packs := make([]KnownPack, packCount)
	for i := 0; i < packCount; i++ {
		var pack KnownPack
		pack.Read(rd)
		packs[i] = pack
	}
	p.Packs = packs
	return nil
}

func (p *KnownPacks) Encode(c *proto.PacketContext, wr io.Writer) error {
	util.PWriteVarInt(wr, len(p.Packs))
	for _, pack := range p.Packs {
		pack.Write(wr)
	}
	return nil
}

type KnownPack struct {
	Namespace string
	Id        string
	Version   string
}

func (p *KnownPack) Write(wr io.Writer) {
	util.PWriteString(wr, p.Namespace)
	util.PWriteString(wr, p.Id)
	util.PWriteString(wr, p.Version)
}

func (p *KnownPack) Read(rd io.Reader) {
	util.PReadString(rd, &p.Namespace)
	util.PReadString(rd, &p.Id)
	util.PReadString(rd, &p.Version)
}

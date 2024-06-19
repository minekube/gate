package packet

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type ServerLinks struct {
	ServerLinks []*ServerLink
}

func (p *ServerLinks) Encode(c *proto.PacketContext, wr io.Writer) error {
	w := protoutil.PanicWriter(wr)
	w.VarInt(len(p.ServerLinks))
	for _, serverLink := range p.ServerLinks {
		err := serverLink.Encode(c, wr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *ServerLinks) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r := protoutil.PanicReader(rd)
	var serverLinksCount int
	r.VarInt(&serverLinksCount)
	p.ServerLinks = make([]*ServerLink, serverLinksCount)
	for i := 0; i < serverLinksCount; i++ {
		p.ServerLinks[i] = new(ServerLink)
		err = p.ServerLinks[i].Decode(c, rd)
		if err != nil {
			return err
		}
	}
	return
}

type ServerLink struct {
	ID          int
	DisplayName chat.ComponentHolder
	URL         string
}

func (p *ServerLink) Encode(c *proto.PacketContext, wr io.Writer) error {
	if p.ID >= 0 {
		protoutil.PWriteBool(wr, true)
		protoutil.PWriteVarInt(wr, p.ID)
	} else {
		protoutil.PWriteBool(wr, false)
		err := p.DisplayName.Write(wr, c.Protocol)
		if err != nil {
			return err
		}
	}
	return protoutil.WriteString(wr, p.URL)
}

func (p *ServerLink) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r := protoutil.PanicReader(rd)
	if r.Ok() {
		r.VarInt(&p.ID)
		r.String(&p.URL)
	} else {
		p.ID = -1
		p.DisplayName, err = chat.ReadComponentHolderNP(rd, c.Protocol)
		if err != nil {
			return err
		}
		r.String(&p.URL)
	}
	return
}

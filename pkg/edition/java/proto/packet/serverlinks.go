package packet

import (
	"fmt"
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
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

	if serverLinksCount < 0 {
		return fmt.Errorf("server links count %d cannot be negative", serverLinksCount)
	}
	const maxServerLinks = 128
	if serverLinksCount > maxServerLinks {
		return fmt.Errorf("too many server links (attempted %d, max %d)", serverLinksCount, maxServerLinks)
	}

	p.ServerLinks = make([]*ServerLink, serverLinksCount)
	for i := 0; i < serverLinksCount; i++ {
		link := new(ServerLink)
		if err = link.Decode(c, rd); err != nil {
			return fmt.Errorf("error decoding server link index %d: %w", i, err)
		}
		p.ServerLinks[i] = link
	}
	return nil
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

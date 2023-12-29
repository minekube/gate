package packet

import (
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/gate/proto"
)

type HeaderAndFooter struct {
	Header chat.ComponentHolder
	Footer chat.ComponentHolder
}

func (h *HeaderAndFooter) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := h.Header.Write(wr, c.Protocol)
	if err != nil {
		return err
	}
	return h.Footer.Write(wr, c.Protocol)
}

// we never read this packet
func (h *HeaderAndFooter) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	h.Header, err = chat.ReadComponentHolderNP(rd, c.Protocol)
	if err != nil {
		return err
	}
	h.Footer, err = chat.ReadComponentHolderNP(rd, c.Protocol)
	return err
}

var ResetHeaderAndFooter = &HeaderAndFooter{
	Header: *chat.FromComponent(new(component.Translation)),
	Footer: *chat.FromComponent(new(component.Translation)),
}

var (
	_ proto.Packet = (*HeaderAndFooter)(nil)
	_ proto.Packet = (*legacytablist.PlayerListItem)(nil)
)

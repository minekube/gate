package packet

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

type HeaderAndFooter struct {
	Header string
	Footer string
}

func (h *HeaderAndFooter) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, h.Header)
	if err != nil {
		return err
	}
	return util.WriteString(wr, h.Footer)
}

// we never read this packet
func (h *HeaderAndFooter) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	h.Header, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	h.Footer, err = util.ReadString(rd)
	return err
}

var ResetHeaderAndFooter = &HeaderAndFooter{
	Header: `{"translate":""}`,
	Footer: `{"translate":""}`,
}

var (
	_ proto.Packet = (*HeaderAndFooter)(nil)
	_ proto.Packet = (*legacytablist.PlayerListItem)(nil)
)

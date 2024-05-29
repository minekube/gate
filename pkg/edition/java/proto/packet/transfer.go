package packet

import (
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
	"io"
	"net"
)

type TransferPacket struct {
	Host string
	Port int
}

// Addr formats the host and port into a net.Addr.
// If the host is empty, the second return value is false.
func (t *TransferPacket) Addr() (net.Addr, bool) {
	if t.Host == "" {
		return nil, false
	}
	return netutil.NewAddr(fmt.Sprintf("%s:%d", t.Host, t.Port), "tcp"), true
}

func (t *TransferPacket) Encode(c *proto.PacketContext, wr io.Writer) error {
	if err := util.WriteString(wr, t.Host); err != nil {
		return err
	}
	return util.WriteVarInt(wr, t.Port)
}

func (t *TransferPacket) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	t.Host, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	t.Port, err = util.ReadVarInt(rd)
	return err
}

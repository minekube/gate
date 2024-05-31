package chat

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type UnsignedPlayerCommand struct {
	SessionPlayerCommand
}

// Signed always returns false as it is unsigned.
func (u *UnsignedPlayerCommand) Signed() bool {
	// note SessionPlayerCommand.Signed() would also return false as Salt is 0
	return false
}

func (u *UnsignedPlayerCommand) Encode(c *proto.PacketContext, wr io.Writer) error {
	return util.WriteString(wr, u.Command)
}

func (u *UnsignedPlayerCommand) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	u.Command, err = util.ReadStringMax(rd, MaxServerBoundMessageLength)
	return err
}

var _ proto.Packet = (*UnsignedPlayerCommand)(nil)

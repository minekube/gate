package chat

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
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
	// UnsignedPlayerCommand always uses 65536 cap since it's only available in 1.20.5+
	u.Command, err = util.ReadStringMax(rd, util.DefaultMaxStringSize)
	return err
}

var _ proto.Packet = (*UnsignedPlayerCommand)(nil)

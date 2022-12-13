package playerinfo

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type Remove struct {
	PlayersToRemove []uuid.UUID
}

func (r *Remove) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, len(r.PlayersToRemove))
	if err != nil {
		return err
	}
	for _, p := range r.PlayersToRemove {
		err = util.WriteUUID(wr, p)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Remove) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	var count int
	count, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	r.PlayersToRemove = make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		r.PlayersToRemove[i], err = util.ReadUUID(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

package cookie

import (
	"io"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

type CookieStore struct {
	Key     key.Key
	Payload []byte
}

func (c *CookieStore) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	if err := util.WriteKey(wr, c.Key); err != nil {
		return err
	}
	return util.WriteBytes(wr, c.Payload)
}

func (c *CookieStore) Decode(ctx *proto.PacketContext, rd io.Reader) (err error) {
	if c.Key, err = util.ReadKey(rd); err != nil {
		return err
	}
	c.Payload, err = util.ReadBytesLen(rd, MaxPayloadSize)
	return err
}

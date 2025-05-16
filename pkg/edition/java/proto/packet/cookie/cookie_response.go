package cookie

import (
	"io"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

const MaxPayloadSize = 5 * 1024 // 5 kiB

type CookieResponse struct {
	Key     key.Key
	Payload []byte
}

func (c *CookieResponse) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	if err := util.WriteKey(wr, c.Key); err != nil {
		return err
	}
	hasPayload := len(c.Payload) > 0
	util.PWriteBool(wr, hasPayload)
	if hasPayload {
		return util.WriteBytes(wr, c.Payload)
	}
	return nil
}

func (c *CookieResponse) Decode(ctx *proto.PacketContext, rd io.Reader) (err error) {
	c.Key, err = util.ReadKey(rd)
	if err != nil {
		return err
	}
	if util.PReadBoolVal(rd) {
		c.Payload, err = util.ReadBytesLen(rd, MaxPayloadSize)
		if err != nil {
			return err
		}
	}
	return nil
}

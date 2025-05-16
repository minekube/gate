package cookie

import (
	"io"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

type CookieRequest struct {
	Key key.Key
}

func (c *CookieRequest) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	return util.WriteKey(wr, c.Key)
}

func (c *CookieRequest) Decode(ctx *proto.PacketContext, rd io.Reader) (err error) {
	c.Key, err = util.ReadKey(rd)
	return err
}

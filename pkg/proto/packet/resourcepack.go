package packet

import (
	"errors"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/util"
	"io"
)

type ResourcePackRequest struct {
	Url  string
	Hash string
}

func (r *ResourcePackRequest) Encode(c *proto.PacketContext, wr io.Writer) error {
	if len(r.Url) == 0 {
		return errors.New("url is missing")
	}
	err := util.WriteString(wr, r.Url)
	if err != nil {
		return err
	}
	return util.WriteString(wr, r.Hash)
}

func (r *ResourcePackRequest) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r.Url, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	r.Hash, err = util.ReadString(rd)
	return
}

var _ proto.Packet = (*ResourcePackRequest)(nil)

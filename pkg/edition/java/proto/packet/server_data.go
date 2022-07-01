package packet

import (
	"io"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/favicon"
)

type ServerData struct {
	Description  component.Component // nil-able
	Favicon      favicon.Favicon     // may be empty
	PreviewsChat bool
}

func (s *ServerData) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteBool(wr, s.Description != nil)
	if err != nil {
		return err
	}
	if s.Description != nil {
		err = util.WriteComponent(wr, c.Protocol, s.Description)
		if err != nil {
			return err
		}
	}
	err = util.WriteBool(wr, s.Favicon != "")
	if err != nil {
		return err
	}
	if s.Favicon != "" {
		err = util.WriteString(wr, string(s.Favicon))
		if err != nil {
			return err
		}
	}
	return util.WriteBool(wr, s.PreviewsChat)
}

func (s *ServerData) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	ok, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		s.Description, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	}
	ok, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		fi, err := util.ReadString(rd)
		if err != nil {
			return err
		}
		s.Favicon = favicon.Favicon(fi)
	}
	s.PreviewsChat, err = util.ReadBool(rd)
	return err
}

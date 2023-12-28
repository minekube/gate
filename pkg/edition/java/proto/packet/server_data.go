package packet

import (
	"io"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/favicon"
)

type ServerData struct {
	Description        component.Component // nil-able
	Favicon            favicon.Favicon     // may be empty
	SecureChatEnforced bool                // Added in 1.19.1
}

func (s *ServerData) Encode(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)
	hasDescription := s.Description != nil
	if c.Protocol.Lower(version.Minecraft_1_19_4) {
		w.Bool(hasDescription)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) || hasDescription {
		err := util.WriteComponent(wr, c.Protocol, s.Description)
		if err != nil {
			return err
		}
	}
	hasFavicon := s.Favicon != ""
	w.Bool(hasFavicon)
	if hasFavicon {
		if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) {
			w.Bytes(s.Favicon.Bytes())
		} else {
			w.String(string(s.Favicon))
		}
	}
	if c.Protocol.Lower(version.Minecraft_1_19_3) {
		w.Bool(false)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		w.Bool(s.SecureChatEnforced)
	}
	return nil
}

func (s *ServerData) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r := util.PanicReader(rd)
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) {
		s.Description, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	} else {
		if r.Ok() {
			s.Description, err = util.ReadComponent(rd, c.Protocol)
			if err != nil {
				return err
			}
		}
	}
	if r.Ok() {
		if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) {
			s.Favicon = favicon.FromBytes(util.PReadBytesVal(rd))
		} else {
			s.Favicon = favicon.Favicon(util.PReadStringVal(rd))
		}
	}
	if c.Protocol.Lower(version.Minecraft_1_19_3) {
		_ = r.Ok()
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		r.Bool(&s.SecureChatEnforced)
	}
	return nil
}

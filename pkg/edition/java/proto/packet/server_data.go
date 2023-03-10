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
	hasDescription := s.Description != nil
	if c.Protocol.Lower(version.Minecraft_1_19_4) {
		err := util.WriteBool(wr, s.Description != nil)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) || hasDescription {
		err := util.WriteComponent(wr, c.Protocol, s.Description)
		if err != nil {
			return err
		}
	}
	hasFavicon := s.Favicon != ""
	err := util.WriteBool(wr, hasFavicon)
	if err != nil {
		return err
	}
	if hasFavicon {
		if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) {
			err = util.WriteBytes(wr, s.Favicon.Bytes())
			if err != nil {
				return err
			}
		} else {
			err = util.WriteString(wr, string(s.Favicon))
			if err != nil {
				return err
			}
		}
	}
	if c.Protocol.Lower(version.Minecraft_1_19_3) {
		err = util.WriteBool(wr, false)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		err = util.WriteBool(wr, s.SecureChatEnforced)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ServerData) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) {
		s.Description, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	} else {
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
	}
	ok, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		if c.Protocol.GreaterEqual(version.Minecraft_1_19_4) {
			b, err := util.ReadBytes(rd)
			if err != nil {
				return err
			}
			s.Favicon = favicon.FromBytes(b)
		} else {
			fi, err := util.ReadString(rd)
			if err != nil {
				return err
			}
			s.Favicon = favicon.Favicon(fi)
		}
	}
	if c.Protocol.Lower(version.Minecraft_1_19_3) {
		_, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		s.SecureChatEnforced, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

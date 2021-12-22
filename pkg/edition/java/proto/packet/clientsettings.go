package packet

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

type ClientSettings struct {
	Locale         string // may be empty
	ViewDistance   byte
	ChatVisibility int
	ChatColors     bool
	Difficulty     bool // 1.7 Protocol
	SkinParts      byte
	MainHand       int
	TextFiltering  bool // 1.17+
	ClientListing  bool // 1.18+, overwrites server-list "anonymous" mode
}

func (s *ClientSettings) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, s.Locale)
	if err != nil {
		return err
	}
	err = util.WriteUint8(wr, s.ViewDistance)
	if err != nil {
		return err
	}
	err = util.WriteVarInt(wr, s.ChatVisibility)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, s.ChatColors)
	if err != nil {
		return err
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_7_6) {
		err = util.WriteBool(wr, s.Difficulty)
		if err != nil {
			return err
		}
	}
	err = util.WriteUint8(wr, s.SkinParts)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_9) {
		err = util.WriteVarInt(wr, s.MainHand)
		if err != nil {
			return err
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_17) {
			err = util.WriteBool(wr, s.TextFiltering)
			if err != nil {
				return err
			}
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
			err = util.WriteBool(wr, s.ClientListing)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ClientSettings) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	s.Locale, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	s.ViewDistance, err = util.ReadUint8(rd)
	if err != nil {
		return err
	}
	s.ChatVisibility, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	s.ChatColors, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_7_6) {
		s.Difficulty, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	s.SkinParts, err = util.ReadByte(rd)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_9) {
		s.MainHand, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_17) {
			s.TextFiltering, err = util.ReadBool(rd)
			if err != nil {
				return err
			}
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
			s.ClientListing, err = util.ReadBool(rd)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

var _ proto.Packet = (*ClientSettings)(nil)

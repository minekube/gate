package packet

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type ClientSettings struct {
	Locale               string // may be empty
	ViewDistance         byte
	ChatVisibility       int
	ChatColors           bool
	Difficulty           byte // 1.7 Protocol
	SkinParts            byte
	MainHand             int
	TextFilteringEnabled bool // 1.17+
	ClientListingAllowed bool // 1.18+, overwrites server-list "anonymous" mode
	ParticleStatus       int  // Added in 1.21.2
}

func (s *ClientSettings) Encode(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)
	w.String(s.Locale)
	w.Byte(s.ViewDistance)
	w.VarInt(s.ChatVisibility)
	w.Bool(s.ChatColors)
	if c.Protocol.LowerEqual(version.Minecraft_1_7_6) {
		w.Byte(s.Difficulty)
	}
	w.Byte(s.SkinParts)
	if c.Protocol.GreaterEqual(version.Minecraft_1_9) {
		w.VarInt(s.MainHand)
		if c.Protocol.GreaterEqual(version.Minecraft_1_17) {
			w.Bool(s.TextFilteringEnabled)
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
			w.Bool(s.ClientListingAllowed)

			if c.Protocol.GreaterEqual(version.Minecraft_1_21_2) {
				w.VarInt(s.ParticleStatus)
			}
		}
	}
	return nil
}

func (s *ClientSettings) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r := util.PanicReader(rd)
	r.StringMax(&s.Locale, 16)
	r.Byte(&s.ViewDistance)
	r.VarInt(&s.ChatVisibility)
	r.Bool(&s.ChatColors)
	if c.Protocol.LowerEqual(version.Minecraft_1_7_6) {
		r.Byte(&s.Difficulty)
	}
	r.Byte(&s.SkinParts) // Go bytes are unsigned already
	if c.Protocol.GreaterEqual(version.Minecraft_1_9) {
		r.VarInt(&s.MainHand)
		if c.Protocol.GreaterEqual(version.Minecraft_1_17) {
			r.Bool(&s.TextFilteringEnabled)
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
			r.Bool(&s.ClientListingAllowed)

			if c.Protocol.GreaterEqual(version.Minecraft_1_21_2) {
				r.VarInt(&s.ParticleStatus)
			}
		}
	}
	return nil
}

var _ proto.Packet = (*ClientSettings)(nil)

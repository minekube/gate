package packet

import (
	"fmt"
	"io"
	"math/rand"
	"strconv"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// SoundSource represents the source/category of a sound
type SoundSource int

const (
	SoundSourceMaster SoundSource = iota
	SoundSourceMusic
	SoundSourceRecord
	SoundSourceWeather
	SoundSourceBlock
	SoundSourceHostile
	SoundSourceNeutral
	SoundSourcePlayer
	SoundSourceAmbient
	SoundSourceVoice
	SoundSourceUI // 1.21.5+
)

// String returns the string representation of the sound source.
func (s SoundSource) String() string {
	if s < 0 || s > SoundSourceUI {
		return fmt.Sprintf("unknown(%d)", s)
	}
	return []string{
		"master", "music", "record", "weather", "block",
		"hostile", "neutral", "player", "ambient", "voice", "ui",
	}[s]
}

func (s *SoundSource) UnmarshalText(text []byte) error {
	switch string(text) {
	case "master":
		*s = SoundSourceMaster
	case "music":
		*s = SoundSourceMusic
	case "record":
		*s = SoundSourceRecord
	case "weather":
		*s = SoundSourceWeather
	case "block":
		*s = SoundSourceBlock
	case "hostile":
		*s = SoundSourceHostile
	case "neutral":
		*s = SoundSourceNeutral
	case "player":
		*s = SoundSourcePlayer
	case "ambient":
		*s = SoundSourceAmbient
	case "voice":
		*s = SoundSourceVoice
	case "ui":
		*s = SoundSourceUI
	default:
		// Try parsing as number
		if num, err := strconv.Atoi(string(text)); err == nil && num >= 0 && num <= 10 {
			*s = SoundSource(num)
			return nil
		}
		return fmt.Errorf("invalid sound source %q. Valid sources: master, music, record, weather, block, hostile, neutral, player, ambient, voice, ui", string(text))
	}
	return nil
}

// SoundEntityPacket is sent to play a sound at an entity's location
type SoundEntityPacket struct {
	SoundID     int
	SoundName   key.Key
	FixedRange  *float32 // optional, if not nil means there's a fixed range
	SoundSource SoundSource
	EntityID    int
	Volume      float32
	Pitch       float32
	Seed        int64
}

var _ proto.Packet = (*SoundEntityPacket)(nil)

func (s *SoundEntityPacket) Encode(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)

	// Write sound ID (hardcoded to 0 for named sounds)
	w.VarInt(s.SoundID)

	if s.SoundID == 0 {
		w.MinimalKey(s.SoundName)

		hasFixedRange := s.FixedRange != nil
		w.Bool(hasFixedRange)
		if hasFixedRange {
			w.Float32(*s.FixedRange)
		}
	}

	if c.Protocol.Lower(version.Minecraft_1_21_5) && s.SoundSource == SoundSourceUI {
		return fmt.Errorf("UI sound-source is only supported in 1.21.5+")
	}
	w.VarInt(int(s.SoundSource))

	w.VarInt(s.EntityID)
	w.Float32(s.Volume)
	w.Float32(s.Pitch)

	seed := s.Seed
	if seed == 0 {
		seed = rand.Int63()
	}
	w.Int64(seed)

	return nil
}

func (s *SoundEntityPacket) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	pr := util.PanicReader(rd)

	pr.VarInt(&s.SoundID)

	if s.SoundID == 0 {
		pr.MinimalKey(&s.SoundName)

		if pr.Ok() {
			var fixedRange float32
			pr.Float32(&fixedRange)
			s.FixedRange = &fixedRange
		} else {
			s.FixedRange = nil
		}
	} else {
		s.SoundName = nil
		s.FixedRange = nil
	}

	var sourceOrdinal int
	pr.VarInt(&sourceOrdinal)
	s.SoundSource = SoundSource(sourceOrdinal)

	if c.Protocol.Lower(version.Minecraft_1_21_5) && s.SoundSource == SoundSourceUI {
		return fmt.Errorf("UI sound-source is only supported in 1.21.5+")
	}

	pr.VarInt(&s.EntityID)
	pr.Float32(&s.Volume)
	pr.Float32(&s.Pitch)
	pr.Int64(&s.Seed)

	return nil
}

// StopSoundPacket is sent to stop playing a sound
type StopSoundPacket struct {
	Source    *SoundSource // nil means stop all sounds from all sources
	SoundName key.Key      // nil means stop all sounds with given source
}

var _ proto.Packet = (*StopSoundPacket)(nil)

func (s *StopSoundPacket) Encode(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)

	var flags byte
	if s.Source != nil {
		flags |= 0x01
	}
	if s.SoundName != nil {
		flags |= 0x02
	}

	w.Byte(flags)

	if s.Source != nil {
		// Check for UI sound source on older versions
		if *s.Source == SoundSourceUI && c.Protocol.Lower(version.Minecraft_1_21_5) {
			// Skip writing UI source on versions that don't support it
			return nil
		}
		w.VarInt(int(*s.Source))
	}

	if s.SoundName != nil {
		w.Key(s.SoundName)
	}

	return nil
}

func (s *StopSoundPacket) Decode(c *proto.PacketContext, rd io.Reader) error {
	pr := util.PanicReader(rd)

	var flags byte
	pr.Byte(&flags)

	if flags&0x01 != 0 {
		var sourceOrdinal int
		pr.VarInt(&sourceOrdinal)

		if c.Protocol.Lower(version.Minecraft_1_21_5) && sourceOrdinal == int(SoundSourceUI) {
			return fmt.Errorf("UI sound-source is only supported in 1.21.5+")
		}
		src := SoundSource(sourceOrdinal)
		s.Source = &src
	}

	if flags&0x02 != 0 {
		pr.Key(&s.SoundName)
	} else {
		s.SoundName = nil
	}

	return nil
}

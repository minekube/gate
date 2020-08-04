package packet

import (
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/util"
	"io"
)

type Respawn struct {
	Dimension            int
	PartialHashedSeed    int64
	Difficulty           int16
	Gamemode             int16
	LevelType            string         // empty by default
	ShouldKeepPlayerData bool           // 1.16+
	DimensionInfo        *DimensionInfo // 1.16+
	PreviousGamemode     int16          // 1.16+
}

func (r *Respawn) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		err = util.WriteString(wr, r.DimensionInfo.RegistryId)
		if err != nil {
			return err
		}
		err = util.WriteString(wr, r.DimensionInfo.LevelName)
		if err != nil {
			return err
		}
	} else {
		err = util.WriteInt32(wr, int32(r.Dimension))
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(proto.Minecraft_1_13_2) {
		err = util.WriteByte(wr, byte(r.Difficulty))
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_15) {
		err = util.WriteInt64(wr, r.PartialHashedSeed)
		if err != nil {
			return err
		}
	}
	err = util.WriteByte(wr, byte(r.Gamemode))
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		err = util.WriteByte(wr, byte(r.PreviousGamemode))
		if err != nil {
			return err
		}
		err = util.WriteBool(wr, r.DimensionInfo.DebugType)
		if err != nil {
			return err
		}
		err = util.WriteBool(wr, r.DimensionInfo.Flat)
		if err != nil {
			return err
		}
		err = util.WriteBool(wr, r.ShouldKeepPlayerData)
		if err != nil {
			return err
		}
	} else {
		err = util.WriteString(wr, r.LevelType)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Respawn) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	var dimensionIdentifier, levelName string
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		dimensionIdentifier, err = util.ReadString(rd)
		if err != nil {
			return err
		}
		levelName, err = util.ReadString(rd)
		if err != nil {
			return err
		}
	} else {
		r.Dimension, err = util.ReadInt(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(proto.Minecraft_1_13_2) {
		difficulty, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		r.Difficulty = int16(difficulty)
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_15) {
		r.PartialHashedSeed, err = util.ReadInt64(rd)
		if err != nil {
			return err
		}
	}
	gamemode, err := util.ReadByte(rd)
	if err != nil {
		return err
	}
	r.Gamemode = int16(gamemode)
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		previousGamemode, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		r.PreviousGamemode = int16(previousGamemode)
		debug, err := util.ReadBool(rd)
		if err != nil {
			return err
		}
		flat, err := util.ReadBool(rd)
		if err != nil {
			return err
		}
		r.DimensionInfo = &DimensionInfo{
			RegistryId: dimensionIdentifier,
			LevelName:  levelName,
			Flat:       flat,
			DebugType:  debug,
		}
		r.ShouldKeepPlayerData, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	} else {
		r.LevelType, err = util.ReadString(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ proto.Packet = (*Respawn)(nil)

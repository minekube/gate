package packet

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type Respawn struct {
	Dimension            int
	PartialHashedSeed    int64
	Difficulty           int16
	Gamemode             int16
	LevelType            string         // empty by default
	DataToKeep           byte           // 1.16+
	DimensionInfo        *DimensionInfo // 1.16-1.16.1
	PreviousGamemode     int16          // 1.16+
	CurrentDimensionData util.NBT       // 1.16.2+
	LastDeathPosition    *DeathPosition // 1.19+
	PortalCooldown       int            // 1.20+
}

func (r *Respawn) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
			err = r.CurrentDimensionData.Write(wr)
			if err != nil {
				return err
			}
			err = util.WriteString(wr, r.DimensionInfo.RegistryIdentifier)
			if err != nil {
				return err
			}
		} else {
			err = util.WriteString(wr, r.DimensionInfo.RegistryIdentifier)
			if err != nil {
				return err
			}
			err = util.WriteString(wr, *r.DimensionInfo.LevelName)
			if err != nil {
				return err
			}
		}
	} else {
		err = util.WriteInt32(wr, int32(r.Dimension))
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		err = util.WriteByte(wr, byte(r.Difficulty))
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		err = util.WriteInt64(wr, r.PartialHashedSeed)
		if err != nil {
			return err
		}
	}
	err = util.WriteByte(wr, byte(r.Gamemode))
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
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
		if c.Protocol.Lower(version.Minecraft_1_19_3) {
			err = util.WriteBool(wr, r.DataToKeep != 0)
			if err != nil {
				return err
			}
		} else if c.Protocol.Lower(version.Minecraft_1_20_2) {
			err = util.WriteByte(wr, r.DataToKeep)
			if err != nil {
				return err
			}
		}
	} else {
		err = util.WriteString(wr, r.LevelType)
		if err != nil {
			return err
		}
	}

	// optional death location
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		r.LastDeathPosition.encode(wr)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		err = util.WriteVarInt(wr, r.PortalCooldown)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_2) {
		err = util.WriteByte(wr, r.DataToKeep)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Respawn) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	var dimensionIdentifier, levelName string
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
			r.CurrentDimensionData, err = util.ReadNBT(rd)
			if err != nil {
				return err
			}
			dimensionIdentifier, err = util.ReadString(rd)
			if err != nil {
				return err
			}
		} else {
			dimensionIdentifier, err = util.ReadString(rd)
			if err != nil {
				return err
			}
			levelName, err = util.ReadString(rd)
			if err != nil {
				return err
			}
		}
	} else {
		r.Dimension, err = util.ReadInt(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		difficulty, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		r.Difficulty = int16(difficulty)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
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
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
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
			RegistryIdentifier: dimensionIdentifier,
			LevelName:          &levelName,
			Flat:               flat,
			DebugType:          debug,
		}

		if c.Protocol.Lower(version.Minecraft_1_19_3) {
			ok, err := util.ReadBool(rd)
			if err != nil {
				return err
			}
			if ok {
				r.DataToKeep = 1
			} else {
				r.DataToKeep = 0
			}
		} else if c.Protocol.Lower(version.Minecraft_1_20_2) {
			r.DataToKeep, err = util.ReadByte(rd)
			if err != nil {
				return err
			}
		}
	} else {
		r.LevelType, err = util.ReadStringMax(rd, 16)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		r.LastDeathPosition, err = decodeDeathPosition(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		r.PortalCooldown, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_2) {
		r.DataToKeep, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ proto.Packet = (*Respawn)(nil)

package packet

import (
	"fmt"
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
	LevelType            string                 // empty by default
	DataToKeep           byte                   // 1.16+
	DimensionInfo        *DimensionInfo         // 1.16-1.16.1
	PreviousGamemode     int16                  // 1.16+
	CurrentDimensionData util.CompoundBinaryTag // 1.16.2+
	LastDeathPosition    *DeathPosition         // 1.19+
	PortalCooldown       int                    // 1.20+
}

func (r *Respawn) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	w := util.PanicWriter(wr)
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
			err = util.WriteBinaryTag(wr, c.Protocol, r.CurrentDimensionData)
			if err != nil {
				return err
			}
			w.String(r.DimensionInfo.RegistryIdentifier)
		} else {
			if c.Protocol.GreaterEqual(version.Minecraft_1_20_5) {
				w.VarInt(r.Dimension)
			} else {
				w.String(r.DimensionInfo.RegistryIdentifier)
			}
			w.String(*r.DimensionInfo.LevelName)
		}
	} else {
		w.Int(r.Dimension)
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		w.Byte(byte(r.Difficulty))
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		w.Int64(r.PartialHashedSeed)
	}
	w.Byte(byte(r.Gamemode))
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		w.Byte(byte(r.PreviousGamemode))
		w.Bool(r.DimensionInfo.DebugType)
		w.Bool(r.DimensionInfo.Flat)
		if c.Protocol.Lower(version.Minecraft_1_19_3) {
			w.Bool(r.DataToKeep != 0)
		} else if c.Protocol.Lower(version.Minecraft_1_20_2) {
			w.Byte(r.DataToKeep)
		}
	} else {
		w.String(r.LevelType)
	}

	// optional death location
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		r.LastDeathPosition.encode(wr)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		w.VarInt(r.PortalCooldown)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_2) {
		w.Byte(r.DataToKeep)
	}
	return nil
}
func (r *Respawn) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	pr := util.PanicReader(rd)
	var dimensionKey, levelName string
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
			r.CurrentDimensionData, err = util.ReadCompoundTag(rd, c.Protocol)
			if err != nil {
				return fmt.Errorf("error reading current dimension data: %w", err)
			}
			pr.String(&dimensionKey)
		} else {
			if c.Protocol.GreaterEqual(version.Minecraft_1_20_5) {
				pr.VarInt(&r.Dimension)
			} else {
				pr.String(&dimensionKey)
			}
			pr.String(&levelName)
		}
	} else {
		pr.Int(&r.Dimension)
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		r.Difficulty = int16(util.PReadByteVal(rd))
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		pr.Int64(&r.PartialHashedSeed)
	}
	r.Gamemode = int16(util.PReadByteVal(rd))
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		r.PreviousGamemode = int16(util.PReadByteVal(rd))
		debug := pr.Ok()
		flat := pr.Ok()
		r.DimensionInfo = &DimensionInfo{
			RegistryIdentifier: dimensionKey,
			LevelName:          &levelName,
			Flat:               flat,
			DebugType:          debug,
		}
		if err = r.DimensionInfo.Validate(c.Protocol); err != nil {
			return err
		}

		if c.Protocol.Lower(version.Minecraft_1_19_3) {
			if pr.Ok() {
				r.DataToKeep = 1
			} else {
				r.DataToKeep = 0
			}
		} else if c.Protocol.Lower(version.Minecraft_1_20_2) {
			pr.Byte(&r.DataToKeep)
		}
	} else {
		pr.String(&r.LevelType)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		r.LastDeathPosition, err = decodeDeathPosition(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		pr.VarInt(&r.PortalCooldown)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_2) {
		pr.Byte(&r.DataToKeep)
	}
	return nil
}

var _ proto.Packet = (*Respawn)(nil)

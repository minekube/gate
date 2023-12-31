package packet

import (
	"errors"
	"fmt"
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type JoinGame struct {
	EntityID             int
	Gamemode             int16
	Dimension            int
	PartialHashedSeed    int64 // 1.15+
	Difficulty           int16
	Hardcore             bool
	MaxPlayers           int
	LevelType            *string // nil-able: removed in 1.16+
	ViewDistance         int     // 1.14+
	ReducedDebugInfo     bool
	ShowRespawnScreen    bool
	DoLimitedCrafting    bool                   // 1.20.2+
	LevelNames           []string               // a set of strings, 1.16+
	Registry             util.CompoundBinaryTag // 1.16+
	DimensionInfo        *DimensionInfo         // 1.16+
	CurrentDimensionData util.CompoundBinaryTag // 1.16.2+
	PreviousGamemode     int16                  // 1.16+
	SimulationDistance   int                    // 1.18+
	LastDeathPosition    *DeathPosition         // 1.19+
	PortalCooldown       int                    // 1.20+
}

type DimensionInfo struct {
	RegistryIdentifier string
	LevelName          *string // nil-able
	Flat               bool
	DebugType          bool
}

type DeathPosition struct {
	Key   string
	Value int64
}

func (d *DeathPosition) encode(wr io.Writer) {
	w := util.PanicWriter(wr)
	w.Bool(d != nil)
	if d != nil {
		w.String(d.Key)
		w.Int64(d.Value)
	}
}

func decodeDeathPosition(rd io.Reader) (*DeathPosition, error) {
	r := util.PanicReader(rd)
	if !r.Ok() {
		return nil, nil
	}
	dp := new(DeathPosition)
	r.String(&dp.Key)
	r.Int64(&dp.Value)
	return dp, nil
}

func (d *DeathPosition) String() string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("%s %d", d.Key, d.Value)
}

func (j *JoinGame) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_2) {
		// they made 1.20.2 more complicated
		return j.encode1202Up(c, wr)
	} else if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		// Minecraft 1.16 and above have significantly more complicated logic for writing this packet,
		// so separate it out.
		return j.encode116Up(c, wr)
	}
	return j.encodeLegacy(c, wr)
}

func (j *JoinGame) encode116Up(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)
	w.Int(j.EntityID)
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		w.Bool(j.Hardcore)
		w.Byte(byte(j.Gamemode))
	} else {
		b := byte(j.Gamemode)
		if j.Hardcore {
			b = byte(j.Gamemode) | 0x8
		}
		w.Byte(b)
	}
	w.Byte(byte(j.PreviousGamemode))
	w.Strings(j.LevelNames)
	w.CompoundBinaryTag(j.Registry, c.Protocol)
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
		w.CompoundBinaryTag(j.CurrentDimensionData, c.Protocol)
		w.String(j.DimensionInfo.RegistryIdentifier)
	} else {
		w.String(j.DimensionInfo.RegistryIdentifier)
		if j.DimensionInfo.LevelName == nil {
			return errors.New("dimension info level name must not be nil")
		}
		w.String(*j.DimensionInfo.LevelName)
	}
	w.Int64(j.PartialHashedSeed)
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		w.VarInt(j.MaxPlayers)
	} else {
		w.Byte(byte(j.MaxPlayers))
	}
	w.VarInt(j.ViewDistance)
	if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
		w.VarInt(j.SimulationDistance)
	}
	w.Bool(j.ReducedDebugInfo)
	w.Bool(j.ShowRespawnScreen)
	w.Bool(j.DimensionInfo.DebugType)
	w.Bool(j.DimensionInfo.Flat)
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		j.LastDeathPosition.encode(wr)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		w.VarInt(j.PortalCooldown)
	}
	return nil
}

func (j *JoinGame) encodeLegacy(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)
	w.Int(j.EntityID)
	b := byte(j.Gamemode)
	if j.Hardcore {
		b = byte(j.Gamemode) | 0x8
	}
	w.Byte(b)
	if c.Protocol.GreaterEqual(version.Minecraft_1_9_1) {
		w.Int(j.Dimension)
	} else {
		w.Byte(byte(j.Dimension))
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		w.Byte(byte(j.Difficulty))
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		w.Int64(j.PartialHashedSeed)
	}
	w.Byte(byte(j.MaxPlayers))
	if j.LevelType == nil {
		return errors.New("no level type specified")
	}
	w.String(*j.LevelType)
	if c.Protocol.GreaterEqual(version.Minecraft_1_14) {
		w.VarInt(j.ViewDistance)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		w.Bool(j.ReducedDebugInfo)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		w.Bool(j.ShowRespawnScreen)
	}
	return nil
}
func (j *JoinGame) encode1202Up(c *proto.PacketContext, wr io.Writer) error {
	w := util.PanicWriter(wr)
	w.Int(j.EntityID)
	w.Bool(j.Hardcore)
	w.Strings(j.LevelNames)
	w.VarInt(j.MaxPlayers)
	w.VarInt(j.ViewDistance)
	w.VarInt(j.SimulationDistance)
	w.Bool(j.ReducedDebugInfo)
	w.Bool(j.ShowRespawnScreen)
	w.Bool(j.DoLimitedCrafting)
	w.String(j.DimensionInfo.RegistryIdentifier)
	if j.DimensionInfo.LevelName == nil {
		return errors.New("dimension info level name must not be nil")
	}
	w.String(*j.DimensionInfo.LevelName)
	w.Int64(j.PartialHashedSeed)
	w.Byte(byte(j.Gamemode))
	w.Byte(byte(j.PreviousGamemode))
	w.Bool(j.DimensionInfo.DebugType)
	w.Bool(j.DimensionInfo.Flat)
	if j.LastDeathPosition != nil {
		w.Bool(true)
		w.String(j.LastDeathPosition.Key)
		w.Int64(j.LastDeathPosition.Value)
	} else {
		w.Bool(false)
	}
	w.VarInt(j.PortalCooldown)
	return nil
}

func (j *JoinGame) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_20_2) {
		// they made 1.20.2 more complicated
		return j.decode1202Up(c, rd)
	} else if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		// Minecraft 1.16 and above have significantly more complicated logic for reading this packet,
		// so separate it out.
		return j.decode116Up(c, rd)
	}
	return j.decodeLegacy(c, rd)
}

func (j *JoinGame) decodeLegacy(c *proto.PacketContext, rd io.Reader) (err error) {
	r := util.PanicReader(rd)
	r.Int(&j.EntityID)
	if err = j.readGamemode(rd); err != nil {
		return err
	}
	j.Hardcore = (j.Gamemode & 0x08) != 0
	j.Gamemode &= ^0x08 // bitwise complement
	if c.Protocol.GreaterEqual(version.Minecraft_1_9_1) {
		r.Int(&j.Dimension)
	} else {
		j.Dimension = int(util.PReadByteVal(rd))
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		j.Difficulty = int16(util.PReadByteVal(rd))
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		r.Int64(&j.PartialHashedSeed)
	}
	j.MaxPlayers = int(util.PReadByteVal(rd))
	lt, err := util.ReadStringMax(rd, 16)
	if err != nil {
		return err
	}
	j.LevelType = &lt
	if c.Protocol.GreaterEqual(version.Minecraft_1_14) {
		r.VarInt(&j.ViewDistance)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		r.Bool(&j.ReducedDebugInfo)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		r.Bool(&j.ShowRespawnScreen)
	}
	return nil
}

func (j *JoinGame) readGamemode(rd io.Reader) (err error) {
	gamemode, err := util.ReadByte(rd)
	j.Gamemode = int16(gamemode)
	return err
}

func (j *JoinGame) decode116Up(c *proto.PacketContext, rd io.Reader) (err error) {
	r := util.PanicReader(rd)
	r.Int(&j.EntityID)
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		r.Bool(&j.Hardcore)
		if err = j.readGamemode(rd); err != nil {
			return err
		}
	} else {
		if err = j.readGamemode(rd); err != nil {
			return err
		}
		j.Hardcore = (j.Gamemode & 0x08) != 0
		j.Gamemode &= ^0x08 // bitwise complement
	}
	j.PreviousGamemode = int16(util.PReadByteVal(rd))

	r.Strings(&j.LevelNames)
	j.Registry, err = util.ReadCompoundTag(rd, c.Protocol)
	if err != nil {
		return fmt.Errorf("error reading registry: %w", err)
	}

	var dimensionIdentifier, levelName string
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
		j.CurrentDimensionData, err = util.ReadCompoundTag(rd, c.Protocol)
		if err != nil {
			return fmt.Errorf("error reading current dimension data: %w", err)
		}
		r.String(&dimensionIdentifier)
	} else {
		r.String(&dimensionIdentifier)
		r.String(&levelName)
	}

	r.Int64(&j.PartialHashedSeed)
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		r.VarInt(&j.MaxPlayers)
	} else {
		j.MaxPlayers = int(util.PReadByteVal(rd))
	}

	r.VarInt(&j.ViewDistance)
	if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
		r.VarInt(&j.SimulationDistance)
	}
	r.Bool(&j.ReducedDebugInfo)
	r.Bool(&j.ShowRespawnScreen)

	debug := r.Ok()
	flat := r.Ok()
	j.DimensionInfo = &DimensionInfo{
		RegistryIdentifier: dimensionIdentifier,
		LevelName:          &levelName,
		Flat:               flat,
		DebugType:          debug,
	}

	// optional death location
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		j.LastDeathPosition, err = decodeDeathPosition(rd)
		if err != nil {
			return err
		}
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		r.VarInt(&j.PortalCooldown)
	}
	return nil
}
func (j *JoinGame) decode1202Up(c *proto.PacketContext, rd io.Reader) error {
	r := util.PanicReader(rd)

	r.Int(&j.EntityID)
	r.Bool(&j.Hardcore)
	r.Strings(&j.LevelNames)
	r.VarInt(&j.MaxPlayers)
	r.VarInt(&j.ViewDistance)
	r.VarInt(&j.SimulationDistance)
	r.Bool(&j.ReducedDebugInfo)
	r.Bool(&j.ShowRespawnScreen)
	r.Bool(&j.DoLimitedCrafting)

	dimensionIdentifier := util.PReadStringVal(rd)
	levelName := util.PReadStringVal(rd)
	r.Int64(&j.PartialHashedSeed)

	j.Gamemode = int16(util.PReadByteVal(rd))
	j.PreviousGamemode = int16(util.PReadByteVal(rd))

	isDebug := r.Ok()
	isFlat := r.Ok()
	j.DimensionInfo = &DimensionInfo{
		RegistryIdentifier: dimensionIdentifier,
		LevelName:          &levelName,
		Flat:               isFlat,
		DebugType:          isDebug,
	}

	// optional death location
	if r.Ok() {
		j.LastDeathPosition = &DeathPosition{
			Key:   util.PReadStringVal(rd),
			Value: util.PReadInt64Val(rd),
		}
	}

	r.VarInt(&j.PortalCooldown)

	return nil
}

var _ proto.Packet = (*JoinGame)(nil)

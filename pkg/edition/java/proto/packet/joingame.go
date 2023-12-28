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
	DoLimitedCrafting    bool           // 1.20.2+
	LevelNames           []string       // a set of strings, 1.16+
	Registry             util.NBT       // 1.16+
	DimensionInfo        *DimensionInfo // 1.16+
	CurrentDimensionData util.NBT       // 1.16.2+
	PreviousGamemode     int16          // 1.16+
	SimulationDistance   int            // 1.18+
	LastDeathPosition    *DeathPosition // 1.19+
	PortalCooldown       int            // 1.20+
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

func (d *DeathPosition) encode(wr io.Writer) error {
	err := util.WriteBool(wr, d != nil)
	if err != nil {
		return err
	}
	if d != nil {
		err = util.WriteString(wr, d.Key)
		if err != nil {
			return err
		}
		err = util.WriteInt64(wr, d.Value)
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeDeathPosition(rd io.Reader) (*DeathPosition, error) {
	ok, err := util.ReadBool(rd)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	dp := new(DeathPosition)
	dp.Key, err = util.ReadString(rd)
	if err != nil {
		return nil, err
	}
	dp.Value, err = util.ReadInt64(rd)
	if err != nil {
		return nil, err
	}
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
	err := util.WriteInt(wr, j.EntityID)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		err = util.WriteBool(wr, j.Hardcore)
		if err != nil {
			return err
		}
		err = util.WriteByte(wr, byte(j.Gamemode))
		if err != nil {
			return err
		}
	} else {
		b := byte(j.Gamemode)
		if j.Hardcore {
			b = byte(j.Gamemode) | 0x8
		}
		err = util.WriteByte(wr, b)
		if err != nil {
			return err
		}
	}
	err = util.WriteByte(wr, byte(j.PreviousGamemode))
	if err != nil {
		return err
	}

	err = util.WriteStrings(wr, j.LevelNames)
	if err != nil {
		return err
	}
	err = j.Registry.Write(wr)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) && c.Protocol.Lower(version.Minecraft_1_19) {
		err = j.CurrentDimensionData.Write(wr)
		if err != nil {
			return err
		}
		err = util.WriteString(wr, j.DimensionInfo.RegistryIdentifier)
		if err != nil {
			return err
		}
	} else {
		err = util.WriteString(wr, j.DimensionInfo.RegistryIdentifier)
		if err != nil {
			return err
		}
		if j.DimensionInfo.LevelName == nil {
			return errors.New("dimension info level name must not be nil")
		}
		err = util.WriteString(wr, *j.DimensionInfo.LevelName)
		if err != nil {
			return err
		}
	}

	err = util.WriteInt64(wr, j.PartialHashedSeed)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		err = util.WriteVarInt(wr, j.MaxPlayers)
		if err != nil {
			return err
		}
	} else {
		err = util.WriteByte(wr, byte(j.MaxPlayers))
		if err != nil {
			return err
		}
	}

	err = util.WriteVarInt(wr, j.ViewDistance)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
		err = util.WriteVarInt(wr, j.SimulationDistance)
		if err != nil {
			return err
		}
	}

	err = util.WriteBool(wr, j.ReducedDebugInfo)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.ShowRespawnScreen)

	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.DimensionInfo.DebugType)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.DimensionInfo.Flat)
	if err != nil {
		return err
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		err = j.LastDeathPosition.encode(wr)
		if err != nil {
			return err
		}
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_20) {
		err = util.WriteVarInt(wr, j.PortalCooldown)
		if err != nil {
			return err
		}
	}

	return nil
}

func (j *JoinGame) encodeLegacy(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt32(wr, int32(j.EntityID))
	if err != nil {
		return err
	}
	b := byte(j.Gamemode)
	if j.Hardcore {
		b = byte(j.Gamemode) | 0x8
	}
	err = util.WriteByte(wr, b)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_9_1) {
		err = util.WriteInt32(wr, int32(j.Dimension))
		if err != nil {
			return err
		}
	} else {
		err = util.WriteByte(wr, byte(j.Dimension))
		if err != nil {
			return err
		}
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		err = util.WriteByte(wr, byte(j.Difficulty))
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		err = util.WriteInt64(wr, j.PartialHashedSeed)
		if err != nil {
			return err
		}
	}
	err = util.WriteByte(wr, byte(j.MaxPlayers))
	if err != nil {
		return err
	}
	if j.LevelType == nil {
		return errors.New("no level type specified")
	}
	err = util.WriteString(wr, *j.LevelType)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_14) {
		err = util.WriteVarInt(wr, j.ViewDistance)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		err = util.WriteBool(wr, j.ReducedDebugInfo)
		if err != nil {
			return err
		}
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		err = util.WriteBool(wr, j.ShowRespawnScreen)
		if err != nil {
			return err
		}
	}
	return nil
}
func (j *JoinGame) encode1202Up(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt(wr, j.EntityID)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.Hardcore)
	if err != nil {
		return err
	}

	err = util.WriteStrings(wr, j.LevelNames)
	if err != nil {
		return err
	}

	err = util.WriteVarInt(wr, j.MaxPlayers)
	if err != nil {
		return err
	}

	err = util.WriteVarInt(wr, j.ViewDistance)
	if err != nil {
		return err
	}
	err = util.WriteVarInt(wr, j.SimulationDistance)
	if err != nil {
		return err
	}

	err = util.WriteBool(wr, j.ReducedDebugInfo)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.ShowRespawnScreen)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.DoLimitedCrafting)
	if err != nil {
		return err
	}

	err = util.WriteString(wr, j.DimensionInfo.RegistryIdentifier)
	if err != nil {
		return err
	}
	if j.DimensionInfo.LevelName == nil {
		return errors.New("dimension info level name must not be nil")
	}
	err = util.WriteString(wr, *j.DimensionInfo.LevelName)
	if err != nil {
		return err
	}
	err = util.WriteInt64(wr, j.PartialHashedSeed)
	if err != nil {
		return err
	}

	err = util.WriteByte(wr, byte(j.Gamemode))
	if err != nil {
		return err
	}
	err = util.WriteByte(wr, byte(j.PreviousGamemode))
	if err != nil {
		return err
	}

	err = util.WriteBool(wr, j.DimensionInfo.DebugType)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, j.DimensionInfo.Flat)
	if err != nil {
		return err
	}

	// optional death location
	if j.LastDeathPosition != nil {
		err = util.WriteBool(wr, true)
		if err != nil {
			return err
		}
		err = util.WriteString(wr, j.LastDeathPosition.Key)
		if err != nil {
			return err
		}
		err = util.WriteInt64(wr, j.LastDeathPosition.Value)
		if err != nil {
			return err
		}
	} else {
		err = util.WriteBool(wr, false)
		if err != nil {
			return err
		}
	}

	err = util.WriteVarInt(wr, j.PortalCooldown)
	if err != nil {
		return err
	}

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
	j.EntityID, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	if err = j.readGamemode(rd); err != nil {
		return err
	}
	j.Hardcore = (j.Gamemode & 0x08) != 0
	j.Gamemode &= ^0x08 // bitwise complement
	if c.Protocol.GreaterEqual(version.Minecraft_1_9_1) {
		j.Dimension, err = util.ReadInt(rd)
		if err != nil {
			return err
		}
	} else {
		d, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		j.Dimension = int(d)
	}
	if c.Protocol.LowerEqual(version.Minecraft_1_13_2) {
		difficulty, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		j.Difficulty = int16(difficulty)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		j.PartialHashedSeed, err = util.ReadInt64(rd)
		if err != nil {
			return err
		}
	}
	maxPlayers, err := util.ReadByte(rd)
	j.MaxPlayers = int(maxPlayers)
	if err != nil {
		return err
	}
	lt, err := util.ReadStringMax(rd, 16)
	if err != nil {
		return err
	}
	j.LevelType = &lt
	if c.Protocol.GreaterEqual(version.Minecraft_1_14) {
		j.ViewDistance, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		j.ReducedDebugInfo, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_15) {
		j.ShowRespawnScreen, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *JoinGame) readGamemode(rd io.Reader) (err error) {
	gamemode, err := util.ReadByte(rd)
	j.Gamemode = int16(gamemode)
	return err
}

func (j *JoinGame) decode116Up(c *proto.PacketContext, rd io.Reader) (err error) {
	j.EntityID, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		j.Hardcore, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
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
	previousGamemode, err := util.ReadByte(rd)
	if err != nil {
		return err
	}
	j.PreviousGamemode = int16(previousGamemode)

	j.LevelNames, err = util.ReadStringArray(rd)
	if err != nil {
		return err
	}
	nbtDecoder := util.NewNBTDecoder(rd)
	j.Registry, err = util.DecodeNBT(nbtDecoder)
	if err != nil {
		return err
	}

	var dimensionIdentifier, levelName string
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) &&
		c.Protocol.Lower(version.Minecraft_1_19) {
		j.CurrentDimensionData, err = util.DecodeNBT(nbtDecoder)
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

	j.PartialHashedSeed, err = util.ReadInt64(rd)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		j.MaxPlayers, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	} else {
		maxPlayers, err := util.ReadByte(rd)
		j.MaxPlayers = int(maxPlayers)
		if err != nil {
			return err
		}
	}

	j.ViewDistance, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_18) {
		j.SimulationDistance, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	j.ReducedDebugInfo, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	j.ShowRespawnScreen, err = util.ReadBool(rd)
	if err != nil {
		return err
	}

	debug, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	flat, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
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
		j.PortalCooldown, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *JoinGame) decode1202Up(c *proto.PacketContext, rd io.Reader) error {
	var err error
	j.EntityID, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	j.Hardcore, err = util.ReadBool(rd)
	if err != nil {
		return err
	}

	j.LevelNames, err = util.ReadStringArray(rd)
	if err != nil {
		return err
	}

	j.MaxPlayers, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}

	j.ViewDistance, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	j.SimulationDistance, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}

	j.ReducedDebugInfo, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	j.ShowRespawnScreen, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	j.DoLimitedCrafting, err = util.ReadBool(rd)
	if err != nil {
		return err
	}

	dimensionIdentifier, err := util.ReadString(rd)
	if err != nil {
		return err
	}
	levelName, err := util.ReadString(rd)
	if err != nil {
		return err
	}
	j.PartialHashedSeed, err = util.ReadInt64(rd)
	if err != nil {
		return err
	}

	gamemode, err := util.ReadByte(rd)
	if err != nil {
		return err
	}
	j.Gamemode = int16(gamemode)

	previousGamemode, err := util.ReadByte(rd)
	if err != nil {
		return err
	}
	j.PreviousGamemode = int16(previousGamemode)

	isDebug, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	isFlat, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	j.DimensionInfo = &DimensionInfo{
		RegistryIdentifier: dimensionIdentifier,
		LevelName:          &levelName,
		Flat:               isFlat,
		DebugType:          isDebug,
	}

	// optional death location
	if ok, err := util.ReadBool(rd); err != nil {
		return err
	} else if ok {
		key, err := util.ReadString(rd)
		if err != nil {
			return err
		}
		value, err := util.ReadInt64(rd)
		if err != nil {
			return err
		}
		j.LastDeathPosition = &DeathPosition{Key: key, Value: value}
	}

	j.PortalCooldown, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}

	return nil
}

var _ proto.Packet = (*JoinGame)(nil)

package packet

import (
	"errors"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
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
	DimensionRegistry    *DimensionRegistry // 1.16+
	DimensionInfo        *DimensionInfo     // 1.16+
	CurrentDimensionData *DimensionData     // 1.16.2+
	PreviousGamemode     int16              // 1.16+
	BiomeRegistry        util.NBT           // 1.16.2+
	SimulationDistance   int                // 1.18+
}

const (
	dimTypeKey = "minecraft:dimension_type"
	biomeKey   = "minecraft:worldgen/biome"
)

func (j *JoinGame) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		// Minecraft 1.16 and above have significantly more complicated logic for writing this packet,
		// so separate it out.
		return j.encode116Up(c, wr)
	}
	return j.encodeLegacy(c, wr)
}

func (j *JoinGame) encode116Up(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt32(wr, int32(j.EntityID))
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
	err = util.WriteStrings(wr, j.DimensionRegistry.LevelNames)
	if err != nil {
		return err
	}
	encodedDimensionRegistry, err := j.DimensionRegistry.encode(c.Protocol)
	if err != nil {
		return err
	}
	registryContainer := util.NBT{}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		registryContainer[dimTypeKey] = util.NBT{
			"type":  dimTypeKey,
			"value": encodedDimensionRegistry,
		}
		if j.BiomeRegistry == nil {
			return errors.New("missing biome registry")
		}
		registryContainer[biomeKey] = j.BiomeRegistry
	} else {
		registryContainer["dimension"] = encodedDimensionRegistry
	}
	nbtEncoder := nbt.NewEncoderWithEncoding(wr, nbt.BigEndian)
	err = nbtEncoder.Encode(registryContainer)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		err = nbtEncoder.Encode(j.CurrentDimensionData.encodeDimensionDetails())
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

func (j *JoinGame) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
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

	levelNames, err := util.ReadStringArray(rd)
	if err != nil {
		return err
	}
	nbtDecoder := nbt.NewDecoderWithEncoding(rd, nbt.BigEndian)
	registryContainer := util.NBT{}
	err = nbtDecoder.Decode(&registryContainer)
	if err != nil {
		return err
	}
	var dimensionRegistryContainer []util.NBT
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		dimType, ok := registryContainer.NBT(dimTypeKey)
		if !ok {
			return dimReadErr("%q not a compound tag, is %T", dimTypeKey, registryContainer[dimTypeKey])
		}
		dimensionRegistryContainer, ok = dimType.List("value")
		if !ok {
			return dimReadErr("%q not an NBT list, is %T", "value", dimType["value"])
		}
		j.BiomeRegistry, ok = registryContainer.NBT(biomeKey)
		if !ok {
			return dimReadErr("%q not a compound tag, is %T", biomeKey, dimType[biomeKey])
		}
	} else {
		var ok bool
		dimensionRegistryContainer, ok = registryContainer.List("dimension")
		if !ok {
			return dimReadErr("%q is not a compound tag list", "dimension")
		}
	}
	readData, err := fromGameData(dimensionRegistryContainer, c.Protocol)
	if err != nil {
		return err
	}
	j.DimensionRegistry = &DimensionRegistry{
		Dimensions: readData,
		LevelNames: levelNames,
	}
	var dimensionIdentifier, levelName string
	if c.Protocol.GreaterEqual(version.Minecraft_1_16_2) {
		currentDimDataTag := util.NBT{}
		err = nbtDecoder.Decode(&currentDimDataTag)
		if err != nil {
			return err
		}
		dimensionIdentifier, err = util.ReadString(rd)
		if err != nil {
			return err
		}
		j.CurrentDimensionData, err = decodeBaseCompoundTag(currentDimDataTag, c.Protocol)
		if err != nil {
			return err
		}
		j.CurrentDimensionData.RegistryIdentifier = dimensionIdentifier
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
	return nil
}

var _ proto.Packet = (*JoinGame)(nil)

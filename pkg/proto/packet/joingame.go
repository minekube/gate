package packet

import (
	"errors"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/util"
	"go.minekube.com/gate/pkg/util/sets"
	"io"
)

type JoinGame struct {
	EntityId          int
	Gamemode          int16
	Dimension         int
	PartialHashedSeed int64 // 1.15+
	Difficulty        int16
	MaxPlayers        int16
	LevelType         *string // nil-able: removed in 1.16+
	ViewDistance      int     // 1.14+
	ReducedDebugInfo  bool
	ShowRespawnScreen bool
	DimensionRegistry *DimensionRegistry // 1.16+
	DimensionInfo     *DimensionInfo     // 1.16+
	PreviousGamemode  int16              // 1.16+
}

func (j *JoinGame) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt32(wr, int32(j.EntityId))
	if err != nil {
		return err
	}
	err = util.WriteByte(wr, byte(j.Gamemode))
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		err = util.WriteByte(wr, byte(j.PreviousGamemode))
		if err != nil {
			return err
		}
		err = util.WriteStrings(wr, j.DimensionRegistry.LevelNames.UnsortedList())
		if err != nil {
			return err
		}
		err = nbt.NewEncoderWithEncoding(wr, nbt.BigEndian).Encode(j.DimensionRegistry.ToNBT())
		if err != nil {
			return err
		}
		err = util.WriteString(wr, j.DimensionInfo.RegistryId)
		if err != nil {
			return err
		}
		err = util.WriteString(wr, j.DimensionInfo.LevelName)
		if err != nil {
			return err
		}
	} else if c.Protocol.GreaterEqual(proto.Minecraft_1_9_1) {
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
	if c.Protocol.LowerEqual(proto.Minecraft_1_13_2) {
		err = util.WriteByte(wr, byte(j.Difficulty))
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_15) {
		err = util.WriteInt64(wr, j.PartialHashedSeed)
		if err != nil {
			return err
		}
	}
	err = util.WriteByte(wr, byte(j.MaxPlayers))
	if err != nil {
		return err
	}
	if c.Protocol.Lower(proto.Minecraft_1_16) {
		if j.LevelType == nil {
			return errors.New("no level type specified")
		}
		err = util.WriteString(wr, *j.LevelType)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_14) {
		err = util.WriteVarInt(wr, j.ViewDistance)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_8) {
		err = util.WriteBool(wr, j.ReducedDebugInfo)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_15) {
		err = util.WriteBool(wr, j.ShowRespawnScreen)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		err = util.WriteBool(wr, j.DimensionInfo.DebugType)
		if err != nil {
			return err
		}
		err = util.WriteBool(wr, j.DimensionInfo.Flat)
		if err != nil {
			return err
		}
	}
	return nil
}

func (j *JoinGame) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	j.EntityId, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	gamemode, err := util.ReadByte(rd)
	if err != nil {
		return err
	}
	j.Gamemode = int16(gamemode)
	var dimensionIdentifier, levelName string
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		previousGamemode, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		j.PreviousGamemode = int16(previousGamemode)
		levelNames, err := util.ReadStringArray(rd)
		if err != nil {
			return err
		}
		var data util.NBT
		if err = nbt.NewDecoderWithEncoding(rd, nbt.BigEndian).Decode(&data); err != nil {
			return err
		}
		readData, err := FromGameData(data)
		if err != nil {
			return err
		}
		j.DimensionRegistry = &DimensionRegistry{
			Dimensions: readData,
			LevelNames: sets.NewString(levelNames...),
		}
		dimensionIdentifier, err = util.ReadString(rd)
		if err != nil {
			return err
		}
		levelName, err = util.ReadString(rd)
		if err != nil {
			return err
		}
	} else if c.Protocol.GreaterEqual(proto.Minecraft_1_9_1) {
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
	if c.Protocol.LowerEqual(proto.Minecraft_1_13_2) {
		difficulty, err := util.ReadByte(rd)
		if err != nil {
			return err
		}
		j.Difficulty = int16(difficulty)
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_15) {
		j.PartialHashedSeed, err = util.ReadInt64(rd)
		if err != nil {
			return err
		}
	}
	maxPlayers, err := util.ReadByte(rd)
	j.MaxPlayers = int16(maxPlayers)
	if err != nil {
		return err
	}
	if c.Protocol.Lower(proto.Minecraft_1_16) {
		lt, err := util.ReadStringMax(rd, 16)
		if err != nil {
			return err
		}
		j.LevelType = &lt
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_14) {
		j.ViewDistance, err = util.ReadVarInt(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_8) {
		j.ReducedDebugInfo, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_15) {
		j.ShowRespawnScreen, err = util.ReadBool(rd)
		if err != nil {
			return err
		}
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		debug, err := util.ReadBool(rd)
		if err != nil {
			return err
		}
		flat, err := util.ReadBool(rd)
		if err != nil {
			return err
		}
		j.DimensionInfo = &DimensionInfo{
			RegistryId: dimensionIdentifier,
			LevelName:  levelName,
			Flat:       flat,
			DebugType:  debug,
		}
	}
	return nil
}

var _ proto.Packet = (*JoinGame)(nil)

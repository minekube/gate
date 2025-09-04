package brigadier

import (
	"fmt"
	"io"
	"strconv"

	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
)

type ArgumentPropertyCodec interface {
	Encode(wr io.Writer, v any, protocol proto.Protocol) error
	Decode(rd io.Reader, protocol proto.Protocol) (any, error)
}

// ArgumentPropertyCodecFuncs implements ArgumentPropertyCodec.
type ArgumentPropertyCodecFuncs struct {
	EncodeFn func(wr io.Writer, v any, protocol proto.Protocol) error
	DecodeFn func(rd io.Reader, protocol proto.Protocol) (any, error)
}

func (c *ArgumentPropertyCodecFuncs) Encode(wr io.Writer, v any, protocol proto.Protocol) error {
	if c.EncodeFn == nil {
		return nil
	}
	return c.EncodeFn(wr, v, protocol)
}

func (c *ArgumentPropertyCodecFuncs) Decode(rd io.Reader, protocol proto.Protocol) (any, error) {
	if c.DecodeFn == nil {
		return nil, nil
	}
	return c.DecodeFn(rd, protocol)
}

var (
	EmptyArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{}

	BoolArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			_, ok := v.(*brigodier.BoolArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.BoolArgumentType but got %T", v)
			}
			return nil
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			return brigodier.Bool, nil
		},
	}
	ByteArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			b, ok := v.(byte)
			if !ok {
				return fmt.Errorf("expected byte but got %T", v)
			}
			return util.WriteByte(wr, b)
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			b, err := util.ReadByte(rd)
			return b, err
		},
	}
	StringArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			t, ok := v.(brigodier.StringType)
			if !ok {
				return fmt.Errorf("expected brigodier.StringType but got %T", v)
			}
			switch t {
			case brigodier.SingleWord, brigodier.QuotablePhase, brigodier.GreedyPhrase:
				return util.WriteVarInt(wr, int(t))
			default:
				return fmt.Errorf("invalid string argument type %d", t)
			}
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			t, err := util.ReadVarInt(rd)
			if err != nil {
				return nil, err
			}
			switch t {
			case 0, 1, 2:
				return brigodier.StringType(t), nil
			default:
				return nil, fmt.Errorf("invalid string argument type %d", t)
			}
		},
	}
	RegistryKeyArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*RegistryKeyArgumentType)
			if !ok {
				return fmt.Errorf("expected *RegistryKeyArgumentType but got %T", v)
			}
			return util.WriteString(wr, i.Identifier)
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			id, err := util.ReadString(rd)
			if err != nil {
				return nil, err
			}
			return &RegistryKeyArgumentType{Identifier: id}, nil
		},
	}
	ResourceOrTagKeyArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*ResourceOrTagKeyArgumentType)
			if !ok {
				return fmt.Errorf("expected *RegistryKeyArgumentType but got %T", v)
			}
			return util.WriteString(wr, i.Identifier)
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			id, err := util.ReadString(rd)
			if err != nil {
				return nil, err
			}
			return &ResourceOrTagKeyArgumentType{Identifier: id}, nil
		},
	}
	ResourceKeyArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*ResourceKeyArgumentType)
			if !ok {
				return fmt.Errorf("expected *ResourceKeyArgumentType but got %T", v)
			}
			return util.WriteString(wr, i.Identifier)
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			id, err := util.ReadString(rd)
			if err != nil {
				return nil, err
			}
			return &ResourceKeyArgumentType{Identifier: id}, nil
		},
	}
	ResourceSelectorArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*ResourceSelectorArgumentType)
			if !ok {
				return fmt.Errorf("expected *ResourceSelectorArgumentType but got %T", v)
			}
			return util.WriteString(wr, i.Identifier)
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			id, err := util.ReadString(rd)
			if err != nil {
				return nil, err
			}
			return &ResourceSelectorArgumentType{Identifier: id}, nil
		},
	}
	TimeArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			if protocol.GreaterEqual(version.Minecraft_1_19_4) {
				i, ok := v.(int)
				if ok {
					return util.WriteInt(wr, i)
				}
			}
			return nil
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			if protocol.GreaterEqual(version.Minecraft_1_19_4) {
				b, err := util.ReadInt(rd)
				return b, err
			}
			return 0, nil
		},
	}

	Float64ArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*brigodier.Float64ArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.Float64ArgumentType but got %T", v)
			}
			hasMin := i.Min != brigodier.MinFloat64
			hasMax := i.Max != brigodier.MaxFloat64
			flag := flags(hasMin, hasMax)

			err := util.WriteByte(wr, flag)
			if err != nil {
				return err
			}
			if hasMin {
				err = util.WriteFloat64(wr, i.Min)
				if err != nil {
					return err
				}
			}
			if hasMax {
				err = util.WriteFloat64(wr, i.Max)
			}
			return err
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			flags, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}
			min := brigodier.MinFloat64
			max := brigodier.MaxFloat64
			if flags&HasMinIntFlag != 0 {
				min, err = util.ReadFloat64(rd)
				if err != nil {
					return nil, err
				}
			}
			if flags&HasMaxIntFlag != 0 {
				max, err = util.ReadFloat64(rd)
				if err != nil {
					return nil, err
				}
			}
			return &brigodier.Float64ArgumentType{Min: min, Max: max}, nil
		},
	}
	Float32ArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*brigodier.Float32ArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.Float32ArgumentType but got %T", v)
			}
			hasMin := i.Min != brigodier.MinFloat32
			hasMax := i.Max != brigodier.MaxFloat32
			flag := flags(hasMin, hasMax)

			err := util.WriteByte(wr, flag)
			if err != nil {
				return err
			}
			if hasMin {
				err = util.WriteFloat32(wr, i.Min)
				if err != nil {
					return err
				}
			}
			if hasMax {
				err = util.WriteFloat32(wr, i.Max)
			}
			return err
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			flags, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}
			min := float32(brigodier.MinFloat32)
			max := float32(brigodier.MaxFloat32)
			if flags&HasMinIntFlag != 0 {
				min, err = util.ReadFloat32(rd)
				if err != nil {
					return nil, err
				}
			}
			if flags&HasMaxIntFlag != 0 {
				max, err = util.ReadFloat32(rd)
				if err != nil {
					return nil, err
				}
			}
			return &brigodier.Float32ArgumentType{Min: min, Max: max}, nil
		},
	}

	Int32ArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*brigodier.Int32ArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.Int32ArgumentType but got %T", v)
			}
			hasMin := i.Min != brigodier.MinInt32
			hasMax := i.Max != brigodier.MaxInt32
			flag := flags(hasMin, hasMax)

			err := util.WriteByte(wr, flag)
			if err != nil {
				return err
			}
			if hasMin {
				err = util.WriteInt32(wr, i.Min)
				if err != nil {
					return err
				}
			}
			if hasMax {
				err = util.WriteInt32(wr, i.Max)
			}
			return err
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			flags, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}
			min := int32(brigodier.MinInt32)
			max := int32(brigodier.MaxInt32)
			if flags&HasMinIntFlag != 0 {
				min, err = util.ReadInt32(rd)
				if err != nil {
					return nil, err
				}
			}
			if flags&HasMaxIntFlag != 0 {
				max, err = util.ReadInt32(rd)
				if err != nil {
					return nil, err
				}
			}
			return &brigodier.Int32ArgumentType{Min: min, Max: max}, nil
		},
	}
	Int64ArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*brigodier.Int64ArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.Int64ArgumentType but got %T", v)
			}
			hasMin := i.Min != brigodier.MinInt64
			hasMax := i.Max != brigodier.MaxInt64
			flag := flags(hasMin, hasMax)

			err := util.WriteByte(wr, flag)
			if err != nil {
				return err
			}
			if hasMin {
				err = util.WriteInt64(wr, i.Min)
				if err != nil {
					return err
				}
			}
			if hasMax {
				err = util.WriteInt64(wr, i.Max)
			}
			return err
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			flags, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}
			min := int64(brigodier.MinInt64)
			max := int64(brigodier.MaxInt64)
			if flags&HasMinIntFlag != 0 {
				min, err = util.ReadInt64(rd)
				if err != nil {
					return nil, err
				}
			}
			if flags&HasMaxIntFlag != 0 {
				max, err = util.ReadInt64(rd)
				if err != nil {
					return nil, err
				}
			}
			return &brigodier.Int64ArgumentType{Min: min, Max: max}, nil
		},
	}
	ModArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			// This is special-cased by ArgumentPropertyRegistry
			return fmt.Errorf("unsupported operation")
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			var identifier *ArgumentIdentifier
			if protocol.GreaterEqual(version.Minecraft_1_19) {
				idx, err := util.ReadVarInt(rd)
				if err != nil {
					return nil, err
				}
				var suffix string
				if idx < 0 {
					suffix = fmt.Sprintf("n%d", -idx)
				} else {
					suffix = strconv.Itoa(idx)
				}
				identifier, err = newArgIdentifier("crossstitch:identified_"+suffix, versionSet{
					version: protocol,
					id:      idx,
				})
				if err != nil {
					return nil, err
				}
			} else {
				id, err := util.ReadString(rd)
				if err != nil {
					return nil, err
				}
				identifier, err = newArgIdentifier(id)
				if err != nil {
					return nil, err
				}
			}

			extraData, err := util.ReadBytes(rd)
			if err != nil {
				return nil, err
			}
			return &ModArgumentProperty{
				Identifier: identifier,
				Data:       extraData,
			}, nil
		},
	}

	EntityArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v any, protocol proto.Protocol) error {
			i, ok := v.(*EntityArgumentType)
			if !ok {
				return fmt.Errorf("expected *EntityArgumentType but got %T", v)
			}
			var b byte
			if i.SingleEntity {
				b = b | 0x1
			}
			if i.OnlyPlayers {
				b = b | 0x2
			}
			return util.WriteByte(wr, b)
		},
		DecodeFn: func(rd io.Reader, protocol proto.Protocol) (any, error) {
			b, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}

			return &EntityArgumentType{SingleEntity: b&0x1 != 0, OnlyPlayers: b&0x2 != 0}, nil
		},
	}
)

const (
	HasMinIntFlag byte = 0x01
	HasMaxIntFlag byte = 0x02
)

func flags(hasMin, hasMax bool) (f byte) {
	if hasMin {
		f |= HasMinIntFlag
	}
	if hasMax {
		f |= HasMaxIntFlag
	}
	return
}

type ModArgumentProperty struct {
	Identifier *ArgumentIdentifier
	Data       []byte
}

func (m *ModArgumentProperty) Parse(*brigodier.StringReader) (any, error) {
	return nil, fmt.Errorf("unsupported operation for %T", m)
}

func (m *ModArgumentProperty) String() string { return "mod" }

var _ brigodier.ArgumentType = (*ModArgumentProperty)(nil)

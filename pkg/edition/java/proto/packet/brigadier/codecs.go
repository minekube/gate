package brigadier

import (
	"fmt"
	"io"

	"go.minekube.com/brigodier"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
)

// ArgumentPropertyCodecFuncs implements ArgumentPropertyCodec.
type ArgumentPropertyCodecFuncs struct {
	EncodeFn func(wr io.Writer, v interface{}) error
	DecodeFn func(rd io.Reader) (interface{}, error)
}

func (c *ArgumentPropertyCodecFuncs) Encode(wr io.Writer, v interface{}) error {
	if c.EncodeFn == nil {
		return nil
	}
	return c.EncodeFn(wr, v)
}

func (c *ArgumentPropertyCodecFuncs) Decode(rd io.Reader) (interface{}, error) {
	if c.DecodeFn == nil {
		return nil, nil
	}
	return c.DecodeFn(rd)
}

var (
	EmptyArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{}

	BoolArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
			_, ok := v.(*brigodier.BoolArgumentType)
			if !ok {
				return fmt.Errorf("execpted *brigodier.BoolArgumentType but got %T", v)
			}
			return nil
		},
		DecodeFn: func(rd io.Reader) (interface{}, error) {
			return brigodier.Bool, nil
		},
	}
	ByteArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
			b, ok := v.(byte)
			if !ok {
				return fmt.Errorf("execpted byte but got %T", v)
			}
			return util.WriteByte(wr, b)
		},
		DecodeFn: func(rd io.Reader) (interface{}, error) {
			return util.ReadByte(rd)
		},
	}
	StringArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
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
		DecodeFn: func(rd io.Reader) (interface{}, error) {
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
		EncodeFn: func(wr io.Writer, v interface{}) error {
			i, ok := v.(*RegistryKeyArgumentType)
			if !ok {
				return fmt.Errorf("expected *RegistryKeyArgumentType but got %T", v)
			}
			return util.WriteString(wr, i.Identifier)
		},
		DecodeFn: func(rd io.Reader) (interface{}, error) {
			id, err := util.ReadString(rd)
			if err != nil {
				return nil, err
			}
			return &RegistryKeyArgumentType{Identifier: id}, nil
		},
	}

	EntityArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
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
		DecodeFn: func(rd io.Reader) (interface{}, error) {
			b, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}

			return &EntityArgumentType{SingleEntity: b&0x1 != 0, OnlyPlayers: b&0x2 != 0}, nil
		},
	}

	Float64ArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
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
		DecodeFn: func(rd io.Reader) (interface{}, error) {
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
		EncodeFn: func(wr io.Writer, v interface{}) error {
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
		DecodeFn: func(rd io.Reader) (interface{}, error) {
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
		EncodeFn: func(wr io.Writer, v interface{}) error {
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
		DecodeFn: func(rd io.Reader) (interface{}, error) {
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
		EncodeFn: func(wr io.Writer, v interface{}) error {
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
		DecodeFn: func(rd io.Reader) (interface{}, error) {
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

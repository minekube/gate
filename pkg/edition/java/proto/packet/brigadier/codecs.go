package brigadier

import (
	"fmt"
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"io"
	"math"
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
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("execpted bool but got %T", v)
			}
			return util.WriteBool(wr, b)
		},
		DecodeFn: func(rd io.Reader) (interface{}, error) {
			return util.ReadBool(rd)
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

	Float64ArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
			i, ok := v.(*brigodier.Float64ArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.Float64ArgumentType but got %T", v)
			}
			hasMin := i.Min != MinFloat64
			hasMax := i.Max != MaxFloat64
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
			min := MinFloat64
			max := MaxFloat64
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

	IntArgumentPropertyCodec ArgumentPropertyCodec = &ArgumentPropertyCodecFuncs{
		EncodeFn: func(wr io.Writer, v interface{}) error {
			i, ok := v.(*brigodier.IntegerArgumentType)
			if !ok {
				return fmt.Errorf("expected *brigodier.IntegerArgumentType but got %T", v)
			}
			hasMin := i.Min != MinInt
			hasMax := i.Max != MaxInt
			flag := flags(hasMin, hasMax)

			err := util.WriteByte(wr, flag)
			if err != nil {
				return err
			}
			if hasMin {
				err = util.WriteInt(wr, i.Min)
				if err != nil {
					return err
				}
			}
			if hasMax {
				err = util.WriteInt(wr, i.Max)
			}
			return err
		},
		DecodeFn: func(rd io.Reader) (interface{}, error) {
			flags, err := util.ReadByte(rd)
			if err != nil {
				return nil, err
			}
			min := MinInt
			max := MaxInt
			if flags&HasMinIntFlag != 0 {
				min, err = util.ReadInt(rd)
				if err != nil {
					return nil, err
				}
			}
			if flags&HasMaxIntFlag != 0 {
				max, err = util.ReadInt(rd)
				if err != nil {
					return nil, err
				}
			}
			return &brigodier.IntegerArgumentType{Min: min, Max: max}, nil
		},
	}
)

const (
	HasMinIntFlag byte = 0x01
	HasMaxIntFlag byte = 0x02

	MinInt = math.MinInt32
	MaxInt = math.MaxInt32

	MinFloat64 = -math.MaxFloat64
	MaxFloat64 = math.MaxFloat64
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

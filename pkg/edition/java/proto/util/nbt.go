package util

import (
	"io"

	"github.com/sandertv/gophertunnel/minecraft/nbt"
)

// NBT is a named binary tag aka compound binary tag.
type NBT map[string]any

func (b NBT) Bool(name string) (bool, bool) {
	val, ok := b.Uint8(name)
	return val == 1, ok
}

func (b NBT) Int8(name string) (ret int8, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(int8)
	}
	return
}
func (b NBT) Uint8(name string) (ret uint8, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(uint8)
	}
	return
}

func (b NBT) Int16(name string) (ret int16, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(int16)
	}
	return
}

func (b NBT) Int32(name string) (ret int32, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(int32)
	}
	return
}
func (b NBT) Int(name string) (int, bool) {
	i, ok := b.Int32(name)
	return int(i), ok
}

func (b NBT) Int64(name string) (ret int64, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(int64)
	}
	return
}

func (b NBT) Float32(name string) (ret float32, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(float32)
	}
	return
}

func (b NBT) Float64(name string) (ret float64, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(float64)
	}
	return
}

func (b NBT) ByteArray(name string) (ret []byte, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.([]byte)
	}
	return
}

func (b NBT) String(name string) (ret string, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(string)
	}
	return
}

func (b NBT) NBT(name string) (ret NBT, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.(map[string]any)
		if !ok {
			ret, ok = val.(NBT)
		}
	}
	return
}

func (b NBT) List(name string) (ret []NBT, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		var l []any
		l, ok = val.([]any)
		if !ok {
			ret, ok = val.([]NBT)
			return
		}
		var n NBT
		for _, e := range l {
			n, ok = e.(map[string]any)
			if ok {
				ret = append(ret, n)
			}
		}
	}
	return
}

func (b NBT) Int32Array(name string) (ret []int32, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.([]int32)
	}
	return
}

func (b NBT) Int64Array(name string) (ret []int64, ok bool) {
	var val any
	if val, ok = b[name]; ok {
		ret, ok = val.([]int64)
	}
	return
}

func ReadNBT(rd io.Reader) (NBT, error) {
	return DecodeNBT(NewNBTDecoder(rd))
}

func DecodeNBT(decoder interface{ Decode(any) error }) (NBT, error) {
	v := NBT{}
	err := decoder.Decode(&v)
	return v, err
}

func NewNBTDecoder(r io.Reader) *nbt.Decoder {
	return nbt.NewDecoderWithEncoding(r, nbt.BigEndian)
}

func NewNBTEncoder(w io.Writer) *nbt.Encoder {
	return nbt.NewEncoderWithEncoding(w, nbt.BigEndian)
}

func WriteNBT(w io.Writer, nbt NBT) error {
	return NewNBTEncoder(w).Encode(nbt)
}

func (b NBT) Write(w io.Writer) error {
	return WriteNBT(w, b)
}

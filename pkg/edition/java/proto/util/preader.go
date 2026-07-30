package util

import (
	"io"

	"go.minekube.com/common/minecraft/key"
)

type PReader struct {
	r io.Reader
}

func PanicReader(r io.Reader) *PReader {
	return &PReader{r}
}

func (r *PReader) VarInt(i *int) {
	PVarInt(r.r, i)
}

func (r *PReader) String(s *string) {
	PReadString(r.r, s)
}

func (r *PReader) StringMax(s *string, max int) {
	PReadStringMax(r.r, s, max)
}

func (r *PReader) Uint8(i *uint8) {
	PReadUint8(r.r, i)
}

func (r *PReader) Bytes(b *[]byte) {
	PReadBytes(r.r, b)
}

func (r *PReader) Bool(b *bool) {
	PReadBool(r.r, b)
}
func (r *PReader) Ok() bool {
	var ok bool
	PReadBool(r.r, &ok)
	return ok
}

func (r *PReader) Int64(i *int64) {
	PReadInt64(r.r, i)
}

func (r *PReader) Int(i *int) {
	PReadInt(r.r, i)
}

func (r *PReader) Strings(i *[]string) {
	PReadStrings(r.r, i)
}

func (r *PReader) Byte(b *byte) {
	PReadByte(r.r, b)
}

func (r *PReader) Float32(f *float32) {
	PReadFloat32(r.r, f)
}

func (r *PReader) Key(k *key.Key) {
	PReadKey(r.r, k)
}

func (r *PReader) MinimalKey(k *key.Key) {
	PReadMinimalKey(r.r, k)
}

func PReadStrings(r io.Reader, i *[]string) {
	v, err := ReadStringArray(r)
	if err != nil {
		panic(err)
	}
	*i = v
}

func PReadInt(r io.Reader, i *int) {
	v, err := ReadInt(r)
	if err != nil {
		panic(err)
	}
	*i = v
}

func PReadInt64(r io.Reader, i *int64) {
	v, err := ReadInt64(r)
	if err != nil {
		panic(err)
	}
	*i = v
}

func PReadBool(r io.Reader, b *bool) {
	v, err := ReadBool(r)
	if err != nil {
		panic(err)
	}
	*b = v
}

func PVarInt(rd io.Reader, i *int) {
	v, err := ReadVarInt(rd)
	if err != nil {
		panic(err)
	}
	*i = v
}

func PReadString(rd io.Reader, s *string) {
	v, err := ReadString(rd)
	if err != nil {
		panic(err)
	}
	*s = v
}

func PReadStringMax(rd io.Reader, s *string, max int) {
	v, err := ReadStringMax(rd, max)
	if err != nil {
		panic(err)
	}
	*s = v
}

func PReadUint8(rd io.Reader, i *uint8) {
	v, err := ReadUint8(rd)
	if err != nil {
		panic(err)
	}
	*i = v
}

func PReadBytes(rd io.Reader, b *[]byte) {
	v, err := ReadBytes(rd)
	if err != nil {
		panic(err)
	}
	*b = v
}

func PReadByte(rd io.Reader, b *byte) {
	v, err := ReadByte(rd)
	if err != nil {
		panic(err)
	}
	*b = v
}

func PReadByteVal(rd io.Reader) byte {
	v, err := ReadByte(rd)
	if err != nil {
		panic(err)
	}
	return v
}

func PReadStringVal(rd io.Reader) string {
	v, err := ReadString(rd)
	if err != nil {
		panic(err)
	}
	return v
}

func PReadInt64Val(rd io.Reader) int64 {
	v, err := ReadInt64(rd)
	if err != nil {
		panic(err)
	}
	return v
}

func PReadIntVal(rd io.Reader) int {
	v, err := ReadInt(rd)
	if err != nil {
		panic(err)
	}
	return v
}

func PReadBytesVal(rd io.Reader) []byte {
	v, err := ReadBytes(rd)
	if err != nil {
		panic(err)
	}
	return v
}

func PReadBoolVal(rd io.Reader) bool {
	v, err := ReadBool(rd)
	if err != nil {
		panic(err)
	}
	return v
}

func PReadFloat32(rd io.Reader, f *float32) {
	v, err := ReadFloat32(rd)
	if err != nil {
		panic(err)
	}
	*f = v
}

func PReadKey(rd io.Reader, k *key.Key) {
	v, err := ReadKey(rd)
	if err != nil {
		panic(err)
	}
	*k = v
}

func PReadMinimalKey(rd io.Reader, k *key.Key) {
	v, err := ReadMinimalKey(rd)
	if err != nil {
		panic(err)
	}
	*k = v
}

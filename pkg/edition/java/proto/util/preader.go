package util

import "io"

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

func PReadBytesVal(rd io.Reader) []byte {
	v, err := ReadBytes(rd)
	if err != nil {
		panic(err)
	}
	return v
}

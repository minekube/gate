package util

import (
	"io"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Recover is a helper function to recover from a panic and set the error pointer to the recovered error.
// If the panic is not an error, it will be re-panicked.
//
// Usage:
//
//	func fn() (err error) {
//		defer Recover(&err)
//		// code that may panic(err)
//	}
func Recover(err *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*err = e
		} else {
			panic(r)
		}
	}
}

// RecoverFunc is a helper function to recover from a panic and set the error pointer to the recovered error.
// If the panic is not an error, it will be re-panicked.
//
// Usage:
//
//	return RecoverFunc(func() error {
//		// code that may panic(err)
//	})
func RecoverFunc(fn func() error) (err error) {
	defer Recover(&err)
	return fn()
}

type PWriter struct {
	w io.Writer
}

func PanicWriter(w io.Writer) *PWriter {
	return &PWriter{w}
}

func (w *PWriter) VarInt(i int) {
	PWriteVarInt(w.w, i)
}

func (w *PWriter) String(s string) {
	PWriteString(w.w, s)
}

func (w *PWriter) Bytes(b []byte) {
	PWriteBytes(w.w, b)
}

func (w *PWriter) Bool(b bool) bool {
	PWriteBool(w.w, b)
	return b
}

func (w *PWriter) Int64(i int64) {
	PWriteInt64(w.w, i)
}

func (w *PWriter) Int(i int) {
	PWriteInt(w.w, i)
}

func (w *PWriter) Byte(b byte) {
	PWriteByte(w.w, b)
}

func (w *PWriter) Strings(s []string) {
	PWriteStrings(w.w, s)
}
func (w *PWriter) CompoundBinaryTag(cbt CompoundBinaryTag, protocol proto.Protocol) {
	PWriteCompoundBinaryTag(w.w, protocol, cbt)
}

func (w *PWriter) Float32(f float32) {
	PWriteFloat32(w.w, f)
}

func (w *PWriter) Key(k key.Key) {
	PWriteKey(w.w, k)
}

func (w *PWriter) MinimalKey(k key.Key) {
	PWriteMinimalKey(w.w, k)
}

func PWriteCompoundBinaryTag(w io.Writer, protocol proto.Protocol, cbt CompoundBinaryTag) {
	if err := WriteBinaryTag(w, protocol, cbt); err != nil {
		panic(err)
	}
}

func PWriteStrings(w io.Writer, s []string) {
	if err := WriteStrings(w, s); err != nil {
		panic(err)
	}
}

func PWriteByte(w io.Writer, b byte) {
	if err := WriteByte(w, b); err != nil {
		panic(err)
	}
}

func PWriteInt(w io.Writer, i int) {
	if err := WriteInt(w, i); err != nil {
		panic(err)
	}
}

func PWriteInt64(w io.Writer, i int64) {
	if err := WriteInt64(w, i); err != nil {
		panic(err)
	}
}

func PWriteBool(w io.Writer, b bool) {
	if err := WriteBool(w, b); err != nil {
		panic(err)
	}
}

func PWriteVarInt(wr io.Writer, i int) {
	if err := WriteVarInt(wr, i); err != nil {
		panic(err)
	}
}
func PWriteString(wr io.Writer, s string) {
	if err := WriteString(wr, s); err != nil {
		panic(err)
	}
}
func PWriteBytes(wr io.Writer, b []byte) {
	if err := WriteBytes(wr, b); err != nil {
		panic(err)
	}
}

func PWriteFloat32(wr io.Writer, f float32) {
	if err := WriteFloat32(wr, f); err != nil {
		panic(err)
	}
}

func PWriteKey(wr io.Writer, k key.Key) {
	if err := WriteKey(wr, k); err != nil {
		panic(err)
	}
}

func PWriteMinimalKey(wr io.Writer, k key.Key) {
	if err := WriteMinimalKey(wr, k); err != nil {
		panic(err)
	}
}

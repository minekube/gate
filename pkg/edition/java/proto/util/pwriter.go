package util

import "io"

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

func (w *PWriter) Bool(b bool) {
	PWriteBool(w.w, b)
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

func (w *PWriter) NBT(nbt NBT) {
	PWriteNBT(w.w, nbt)
}

func PWriteNBT(w io.Writer, nbt NBT) {
	if err := WriteNBT(w, nbt); err != nil {
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

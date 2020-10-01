package util

import (
	"errors"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"io"
)

// ReadVarUint32 reads up to 5 bytes from the source buffer passed and sets the integer produced to a pointer.
func ReadVarUint32(r io.Reader) (uint32, error) {
	var v uint32
	for i := uint(0); i < 35; i += 7 {
		b, err := util.ReadByte(r)
		if err != nil {
			return 0, err
		}
		v |= uint32(b&0x7f) << i
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, errors.New("varuint32 did not terminate after 5 bytes")
}

// WriteVarUint32 writes a uint32 to the destination buffer passed with a size of 1-5 bytes.
func WriteVarUint32(w io.Writer, x uint32) error {
	for x >= 0x80 {
		if err := util.WriteByte(w, byte(x)|0x80); err != nil {
			return err
		}
		x >>= 7
	}
	if err := util.WriteByte(w, byte(x)); err != nil {
		return err
	}
	return nil
}

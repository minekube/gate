package util

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
	"io/ioutil"
	"math"
)

func ReadString(rd io.Reader) (string, error) {
	return ReadStringLen(rd, bufio.MaxScanTokenSize)
}

func ReadStringLen(rd io.Reader, maxLength int) (string, error) {
	b, err := ReadBytesLen(rd, maxLength)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ReadStringArray(rd io.Reader) ([]string, error) {
	b, err := ReadBytes(rd)
	if err != nil {
		return nil, err
	}
	s := make([]string, 0, len(b))
	for _, e := range b {
		s = append(s, string(e))
	}
	return s, nil
}

func ReadBytes(rd io.Reader) ([]byte, error) {
	return ReadBytesLen(rd, bufio.MaxScanTokenSize)
}

func ReadBytesLen(rd io.Reader, maxLength int) (bytes []byte, err error) {
	length, err := ReadVarInt(rd)
	if err != nil {
		return
	}
	if length < 0 {
		err = fmt.Errorf("decode, bytes/string length is < 0: %d", length)
		return
	}
	if length > maxLength {
		err = fmt.Errorf("decode, bytes/string length is above given maximum: %d", maxLength)
		return
	}
	bytes = make([]byte, length)
	_, err = rd.Read(bytes)
	return
}

// Reads a non length-prefixed string from the Reader.
// We need this for the legacy 1.7 version, being inconsistent when sending the plugin message channel brand.
func ReadStringWithoutLen(rd io.Reader) (string, error) {
	b, err := ioutil.ReadAll(rd)
	return string(b), err
}

func ReadVarInt(r io.Reader) (result int, err error) {
	if br, ok := r.(io.ByteReader); ok {
		var n uint32
		for i := 0; ; i++ {
			sec, err := br.ReadByte()
			if err != nil {
				return 0, err
			}

			n |= uint32(sec&0x7F) << uint32(7*i)

			if i >= 5 {
				return 0, errors.New("decode: VarInt is too big")
			} else if sec&0x80 == 0 {
				break
			}
		}
		return int(n), nil
	}

	var bytes byte = 0
	var b byte
	var uresult uint32 = 0
	for {
		b, err = ReadUint8(r)
		if err != nil {
			return
		}
		uresult |= uint32(b&0x7F) << uint32(bytes*7)
		bytes++
		if bytes > 5 {
			err = errors.New("decode: VarInt is too big")
			return
		}
		if (b & 0x80) == 0x80 {
			continue
		}
		break
	}
	result = int(int32(uresult))
	return
}

func ReadBool(reader io.Reader) (val bool, err error) {
	uval, err := ReadUint8(reader)
	if err != nil {
		return
	}
	val = uval != 0
	return
}

func ReadInt8(reader io.Reader) (val int8, err error) {
	uval, err := ReadUint8(reader)
	val = int8(uval)
	return
}

func ReadUint8(reader io.Reader) (val uint8, err error) {
	if br, ok := reader.(io.ByteReader); ok {
		return br.ReadByte()
	}
	var protocol [1]byte
	_, err = reader.Read(protocol[:1])
	val = protocol[0]
	return
}

func ReadByte(reader io.Reader) (val byte, err error) {
	return ReadUint8(reader)
}

func ReadInt16(reader io.Reader) (val int16, err error) {
	uval, err := ReadUint16(reader)
	val = int16(uval)
	return
}

func ReadUint16(reader io.Reader) (val uint16, err error) {
	var protocol [2]byte
	_, err = reader.Read(protocol[:2])
	val = binary.BigEndian.Uint16(protocol[:2])
	return
}

func ReadInt32(reader io.Reader) (val int32, err error) {
	uval, err := ReadUint32(reader)
	val = int32(uval)
	return
}

func ReadInt(rd io.Reader) (int, error) {
	i, err := ReadInt32(rd)
	return int(i), err
}

func ReadUint32(reader io.Reader) (val uint32, err error) {
	var protocol [4]byte
	_, err = reader.Read(protocol[:4])
	val = binary.BigEndian.Uint32(protocol[:4])
	return
}

func ReadInt64(reader io.Reader) (val int64, err error) {
	uval, err := ReadUint64(reader)
	val = int64(uval)
	return
}

func ReadUint64(reader io.Reader) (val uint64, err error) {
	var protocol [8]byte
	_, err = reader.Read(protocol[:8])
	val = binary.BigEndian.Uint64(protocol[:8])
	return
}

func ReadFloat32(reader io.Reader) (val float32, err error) {
	ival, err := ReadUint32(reader)
	val = math.Float32frombits(ival)
	return
}

func ReadFloat64(reader io.Reader) (val float64, err error) {
	ival, err := ReadUint64(reader)
	val = math.Float64frombits(ival)
	return
}

// ReadExtendedForgeShort reads a Minecraft-style extended short from the specified {@code buf}.
func ReadExtendedForgeShort(rd io.Reader) (int, error) {
	ulow, err := ReadUint8(rd)
	if err != nil {
		return 0, err
	}
	low := int(ulow)
	var high int
	if low&(0x8000) != 0 {
		low = low & 0x7FFF
		uhigh, err := ReadUint8(rd)
		if err != nil {
			return 0, err
		}
		high = int(uhigh)
	}

	return ((high & 0xFF) << 15) | low, nil
}

const ForgeMaxArrayLength = math.MaxInt32 & 0x1FFF9A

// ReadRetainedByteBufSlice17 reads bytes with the Minecraft 1.7 style length.
func ReadBytes17(rd io.Reader) ([]byte, error) {
	// Read in a 2 or 3 byte number that represents the length of the packet.
	// (3 byte "shorts" for Forge only)
	// No vanilla packet should give a 3 byte packet
	length, err := ReadExtendedForgeShort(rd)
	if err != nil {
		return nil, err
	}
	if length > ForgeMaxArrayLength {
		return nil, fmt.Errorf("cannot receive array > %d (got %d)", ForgeMaxArrayLength, length)
	}

	b := make([]byte, length)
	_, err = rd.Read(b)
	return b, err
}

func ReadUuid(rd io.Reader) (id uuid.UUID, err error) {
	// there is probably a simpler method to convert two int64 to [16]byte uuid

	//high, err := ReadInt(rd)
	//msbHigh := int64(high) << 32
	//
	//low, err := ReadInt(rd)
	//msbLow := int64(low) & 0xFFFFFFFF
	//
	//msb := msbHigh | msbLow
	//
	//high, err := ReadInt(rd)
	//lsbHigh := int64(high) << 32
	//
	//low, err = ReadInt(rd)
	//lsbLow := int64(low) & 0xFFFFFFFF
	//
	//lsb := lsbHigh | lsbLow

	l1, err := ReadInt64(rd)
	if err != nil {
		return uuid.UUID{}, err
	}
	l2, err := ReadInt64(rd)
	if err != nil {
		return uuid.UUID{}, err
	}

	b := make([]byte, 16)
	binary.BigEndian.PutUint64(b, uint64(l1))
	binary.BigEndian.PutUint64(b, uint64(l2))

	copy(id[:], b)
	return
}

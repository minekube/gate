package util

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
	"io/ioutil"
	"math"
)

func ReadString(rd io.Reader) (string, error) {
	return ReadStringMax(rd, bufio.MaxScanTokenSize)
}

func ReadStringMax(rd io.Reader, max int) (string, error) {
	length, err := ReadVarInt(rd)
	if err != nil {
		return "", err
	}
	return readStringMax(rd, max, length)
}

func readStringMax(rd io.Reader, max, length int) (string, error) {
	if length < 0 {
		return "", errors.New("length of string must not 0")
	}
	if length > max*4 { // *4 since UTF8 character has up to 4 bytes
		return "", fmt.Errorf("bad string length (got %d, max. %d)", length, max)
	}
	str := make([]byte, length)
	_, err := io.ReadFull(rd, str)
	if err != nil {
		return "", err
	}
	return string(str), nil
}

func ReadStringArray(rd io.Reader) ([]string, error) {
	length, err := ReadVarInt(rd)
	if err != nil {
		return nil, err
	}
	a := make([]string, 0, length)
	for i := 0; i < length; i++ {
		s, err := ReadString(rd)
		if err != nil {
			return nil, err
		}
		a = append(a, s)
	}
	return a, nil
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
		err = fmt.Errorf("decode, bytes/string length %d is above given maximum: %d", length, maxLength)
		return
	}
	bytes = make([]byte, length)
	_, err = rd.Read(bytes)
	return
}

// ReadStringWithoutLen reads a non length-prefixed string from the Reader.
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

func ReadIntArray(rd io.Reader) ([]int, error) {
	length, err := ReadVarInt(rd)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, fmt.Errorf("got negative-length int array (%d)", length)
	}
	a := make([]int, length)
	for i := 0; i < length; i++ {
		a[i], err = ReadVarInt(rd)
		if err != nil {
			return nil, err
		}
	}
	return a, nil
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

// ReadBytes17 reads bytes with the Minecraft 1.7 style length.
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

func ReadUUID(rd io.Reader) (id uuid.UUID, err error) {
	b := make([]byte, 16)
	_, err = io.ReadFull(rd, b)
	if err != nil {
		return
	}
	return uuid.FromBytes(b)
}

func ReadProperties(rd io.Reader) (props []profile.Property, err error) {
	var size int
	size, err = ReadVarInt(rd)
	if err != nil {
		return
	}
	props = make([]profile.Property, 0, size)
	var name, value, signature string
	for i := 0; i < size; i++ {
		name, err = ReadString(rd)
		if err != nil {
			return
		}
		value, err = ReadString(rd)
		if err != nil {
			return
		}
		signature = ""
		hasSignature, err := ReadBool(rd)
		if err != nil {
			return nil, err
		}
		if hasSignature {
			signature, err = ReadString(rd)
			if err != nil {
				return nil, err
			}
		}
		props = append(props, profile.Property{
			Name:      name,
			Value:     value,
			Signature: signature,
		})
	}
	return
}

//
//
//
//
//

// ReadUTF util function as exists in Java
func ReadUTF(rd io.Reader) (string, error) {
	length, err := ReadUint16(rd)
	if err != nil {
		return "", err
	}
	p := make([]byte, length)
	_, err = io.ReadFull(rd, p)
	return string(p), err
}

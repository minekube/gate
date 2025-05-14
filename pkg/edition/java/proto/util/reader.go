package util

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"go.minekube.com/common/minecraft/key"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	DefaultMaxStringSize = bufio.MaxScanTokenSize
)

func ReadString(rd io.Reader) (string, error) {
	return ReadStringMax(rd, DefaultMaxStringSize)
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
	return ReadBytesLen(rd, DefaultMaxStringSize)
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

// Reads a non length prefixed string from the reader. This is necessary for parsing
// certain packets like the velocity login/hello packet (no length prefix).
func ReadRawBytes(rd io.Reader) ([]byte, error) {
	return io.ReadAll(rd)
}

// ReadStringWithoutLen reads a non length-prefixed string from the Reader.
// We need this for the legacy 1.7 version, being inconsistent when sending the plugin message channel brand.
func ReadStringWithoutLen(rd io.Reader) (string, error) {
	b, err := io.ReadAll(rd)
	return string(b), err
}

// ReadVarInt reads a varint from the reader.
func ReadVarInt(r io.Reader) (result int, err error) {
	result, _, err = ReadVarIntReturnN(r)
	return
}

// ReadVarIntReturnN reads a varint from the reader and returns the number of bytes read.
func ReadVarIntReturnN(r io.Reader) (result int, n int, err error) {
	if br, ok := r.(io.ByteReader); ok {
		var val uint32
		for i := 0; ; i++ {
			sec, err := br.ReadByte()
			if err != nil {
				return 0, i, err
			}

			val |= uint32(sec&0x7F) << uint32(7*i)

			if i >= 5 {
				return 0, 5, errors.New("decode: VarInt is too big")
			} else if sec&0x80 == 0 {
				n = i + 1
				break
			}
		}
		return int(val), n, nil
	}

	var bytesRead byte = 0
	var b byte
	var uresult uint32 = 0
	for {
		b, err = ReadUint8(r)
		if err != nil {
			return 0, int(bytesRead), err
		}
		uresult |= uint32(b&0x7F) << uint32(bytesRead*7)
		bytesRead++
		if bytesRead > 5 {
			err = errors.New("decode: VarInt is too big")
			return 0, int(bytesRead), err
		}
		if (b & 0x80) == 0x80 {
			continue
		}
		break
	}
	result = int(int32(uresult))
	return result, int(bytesRead), nil
}

// ReadVarIntArray reads a VarInt array from the reader.
func ReadVarIntArray(rd io.Reader) ([]int, error) {
	length, err := ReadVarInt(rd)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, fmt.Errorf("got a negative-length array (%d)", length)
	}
	array := make([]int, length)
	for i := 0; i < length; i++ {
		array[i], err = ReadVarInt(rd)
		if err != nil {
			return nil, err
		}
	}
	return array, nil
}

// WriteVarIntArray writes a variable-length integer array to the writer.
func WriteVarIntArray(wr io.Writer, array []int) error {
	err := WriteVarInt(wr, len(array))
	if err != nil {
		return err
	}
	for _, value := range array {
		err = WriteVarInt(wr, value)
		if err != nil {
			return err
		}
	}
	return nil
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

func ReadUUIDIntArray(rd io.Reader) (uuid.UUID, error) {
	msbHigh, err := ReadInt(rd)
	if err != nil {
		return uuid.UUID{}, err
	}
	msbLow, err := ReadInt(rd)
	if err != nil {
		return uuid.UUID{}, err
	}
	lsbHigh, err := ReadInt(rd)
	if err != nil {
		return uuid.UUID{}, err
	}
	lsbLow, err := ReadInt(rd)
	if err != nil {
		return uuid.UUID{}, err
	}
	msb := int64(msbHigh)<<32 | int64(msbLow)&0xFFFFFFFF
	lsb := int64(lsbHigh)<<32 | int64(lsbLow)&0xFFFFFFFF

	return uuid.FromBytes(append(int64ToBytes(msb), int64ToBytes(lsb)...))
}

func int64ToBytes(i int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(i))
	return buf[:]
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

func ReadUnixMilli(rd io.Reader) (time.Time, error) {
	milli, err := ReadInt64(rd)
	return time.UnixMilli(milli), err
}

//
//
//
//
//

const defaultKeySeparator = ":"

// ReadKey reads a standard Mojang Text namespaced:key from the reader.
func ReadKey(rd io.Reader) (key.Key, error) {
	str, err := ReadString(rd)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(str, defaultKeySeparator, 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid key format")
	}
	return key.New(parts[0], parts[1]), nil
}

// WriteKey writes a standard Mojang Text namespaced:key to the writer.
func WriteKey(wr io.Writer, k key.Key) error {
	return WriteString(wr, k.String())
}

// ReadKeyArray reads a standard Mojang Text namespaced:key array from the reader.
func ReadKeyArray(rd io.Reader) ([]key.Key, error) {
	length, err := ReadVarInt(rd)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, fmt.Errorf("got a negative-length array (%d)", length)
	}
	keys := make([]key.Key, length)
	for i := 0; i < length; i++ {
		keys[i], err = ReadKey(rd)
		if err != nil {
			return nil, err
		}
	}
	return keys, nil
}

// WriteKeyArray writes a standard Mojang Text namespaced:key array to the writer.
func WriteKeyArray(wr io.Writer, keys []key.Key) error {
	err := WriteVarInt(wr, len(keys))
	if err != nil {
		return err
	}
	for _, k := range keys {
		err = WriteKey(wr, k)
		if err != nil {
			return err
		}
	}
	return nil
}

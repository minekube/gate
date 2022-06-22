package util

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/util/uuid"
)

func WriteString(writer io.Writer, val string) (err error) {
	return WriteBytes(writer, []byte(val))
}

func WriteVarInt(writer io.Writer, val int) (err error) {
	uval := uint32(val)
	for uval >= 0x80 {
		err = WriteUint8(writer, byte(uval)|0x80)
		if err != nil {
			return
		}
		uval >>= 7
	}
	err = WriteUint8(writer, byte(uval))
	return
}

func WriteBool(writer io.Writer, val bool) (err error) {
	if val {
		err = WriteUint8(writer, 1)
	} else {
		err = WriteUint8(writer, 0)
	}
	return
}

// equal to WriteUint8
func WriteInt8(writer io.Writer, val int8) (err error) {
	return WriteUint8(writer, uint8(val))
}

// equal to WriteByte
func WriteUint8(writer io.Writer, val uint8) (err error) {
	var protocol [1]byte
	protocol[0] = val
	_, err = writer.Write(protocol[:1])
	return
}

// equal to WriteUint8
func WriteByte(writer io.Writer, val byte) (err error) {
	return WriteUint8(writer, val)
}

func WriteInt16(writer io.Writer, val int16) (err error) {
	err = WriteUint16(writer, uint16(val))
	return
}

func WriteUint16(writer io.Writer, val uint16) (err error) {
	var protocol [2]byte
	binary.BigEndian.PutUint16(protocol[:2], val)
	_, err = writer.Write(protocol[:2])
	return
}

func WriteInt32(writer io.Writer, val int32) (err error) {
	err = WriteUint32(writer, uint32(val))
	return
}

func WriteInt(writer io.Writer, val int) (err error) {
	return WriteInt32(writer, int32(val))
}

func WriteUint32(writer io.Writer, val uint32) (err error) {
	var protocol [4]byte
	binary.BigEndian.PutUint32(protocol[:4], val)
	_, err = writer.Write(protocol[:4])
	return
}

func WriteInt64(writer io.Writer, val int64) (err error) {
	err = WriteUint64(writer, uint64(val))
	return
}

func WriteUint64(writer io.Writer, val uint64) (err error) {
	var protocol [8]byte
	binary.BigEndian.PutUint64(protocol[:8], val)
	_, err = writer.Write(protocol[:8])
	return
}

func WriteFloat32(writer io.Writer, val float32) (err error) {
	return WriteUint32(writer, math.Float32bits(val))
}

func WriteFloat64(writer io.Writer, val float64) (err error) {
	return WriteUint64(writer, math.Float64bits(val))
}

func WriteBytes(wr io.Writer, b []byte) (err error) {
	err = WriteVarInt(wr, len(b))
	if err != nil {
		return err
	}
	_, err = wr.Write(b)
	return err
}

// Writes a raw strema of bytes to a file with no length prefix.
// Necessary for the Velocity hello/login packet (it uses a non-standard packet format)
func WriteRawBytes(wr io.Writer, b []byte) (err error) {
	_, err = wr.Write(b)
	return err
}

func WriteStrings(wr io.Writer, a []string) error {
	err := WriteVarInt(wr, len(a))
	if err != nil {
		return err
	}
	for _, s := range a {
		err = WriteString(wr, s)
		if err != nil {
			return err
		}
	}
	return nil
}

// Encoded as an unsigned 128-bit integer
// (or two unsigned 64-bit integers: the most
// significant 64 bits and then the least significant 64 bits)
func WriteUUID(wr io.Writer, uuid uuid.UUID) error {
	err := WriteUint64(wr, binary.BigEndian.Uint64(uuid[:8]))
	if err != nil {
		return err
	}
	return WriteUint64(wr, binary.BigEndian.Uint64(uuid[8:]))
}

func WriteProperties(wr io.Writer, properties []profile.Property) error {
	err := WriteVarInt(wr, len(properties))
	if err != nil {
		return err
	}
	for _, p := range properties {
		err = WriteString(wr, p.Name)
		if err != nil {
			return err
		}
		err = WriteString(wr, p.Value)
		if err != nil {
			return err
		}
		if len(p.Signature) != 0 {
			err = WriteBool(wr, true)
			if err != nil {
				return err
			}
			err = WriteString(wr, p.Signature)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

/*
func WriteUUIDIntArray(wr io.Writer, uuid [16]byte) (err error) {
	err = WriteInt32(wr, ByteArrayToInt32(uuid[:3]))
	if err != nil {
		return err
	}
	err = WriteInt32(wr, ByteArrayToInt32(uuid[4:7]))
	if err != nil {
		return err
	}
	err = WriteInt32(wr, ByteArrayToInt32(uuid[8:10]))
	if err != nil {
		return err
	}
	err = WriteInt32(wr, ByteArrayToInt32(uuid[11:13]))
	if err != nil {
		return err
	}
	return
}

func Int32ToByteArray(num int32) []byte {
	size := int(unsafe.Sizeof(num))
	arr := make([]byte, size)
	for i := 0 ; i < size ; i++ {
		byt := *(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(&num)) + uintptr(i)))
		arr[i] = byt
	}
	return arr
}

func ByteArrayToInt32(arr []byte) int32{
	val := int32(0)
	size := len(arr)
	for i := 0 ; i < size ; i++ {
		*(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(&val)) + uintptr(i))) = arr[i]
	}
	return val
}*/

func WriteBytes17(wr io.Writer, b []byte, allowExtended bool) error {
	if allowExtended {
		if len(b) > ForgeMaxArrayLength {
			return fmt.Errorf("cannot write byte array longer than %d (got %d bytes)",
				ForgeMaxArrayLength, len(b))
		}
	} else {
		if len(b) > math.MaxInt8 {
			return fmt.Errorf("cannot write byte array longer than %d (got %d bytes)",
				math.MaxInt8, len(b))
		}
	}
	// Writes a 2 or 3 byte number that represents the length of the packet. (3 byte "shorts" for
	// Forge only)
	// No vanilla packet should give a 3 byte packet, this method will still retain vanilla
	// behaviour.
	err := WriteExtendedForgeShort(wr, len(b))
	if err != nil {
		return err
	}
	_, err = wr.Write(b)
	return err
}

func WriteExtendedForgeShort(wr io.Writer, toWrite int) (err error) {
	low := toWrite & 0x7FFF
	high := (toWrite & 0x7F8000) >> 15
	if high != 0 {
		low = low | 0x8000
	}
	if err = WriteInt8(wr, int8(low)); err != nil {
		return err
	}
	if high != 0 {
		_, err = wr.Write([]byte{byte(high)})
	}
	return
}

// WriteUTF util function as exists in Java
func WriteUTF(wr io.Writer, s string) error {
	err := WriteUint16(wr, uint16(len(s)))
	if err != nil {
		return err
	}
	_, err = wr.Write([]byte(s))
	return err
}

func WritePlayerKey(wr io.Writer, playerKey crypto.IdentifiedKey) error {
	err := WriteInt64(wr, playerKey.ExpiryTemporal().UnixMilli())
	if err != nil {
		return err
	}
	err = WriteBytes(wr, playerKey.SignedPublicKey().N.Bytes())
	if err != nil {
		return err
	}
	return WriteBytes(wr, playerKey.Signature())
}

package chat

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/mathutil"
)

type LastSeenMessages struct {
	Offset       int
	Acknowledged mathutil.BitSet
	Checksum     byte
}

var _ proto.Packet = (*LastSeenMessages)(nil)

func (l *LastSeenMessages) Encode(c *proto.PacketContext, wr io.Writer) error {
	if err := util.WriteVarInt(wr, l.Offset); err != nil {
		return err
	}
	if _, err := wr.Write(copyOf(l.Acknowledged)); err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_21_5) {
		if err := util.WriteByte(wr, l.Checksum); err != nil {
			return err
		}
	}
	return nil
}

func (l *LastSeenMessages) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	l.Offset, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	acknowledged := make([]byte, divFloor)
	if _, err = io.ReadFull(rd, acknowledged); err != nil {
		return err
	}
	l.Acknowledged = mathutil.BitSet{Bytes: acknowledged}
	if c.Protocol.GreaterEqual(version.Minecraft_1_21_5) {
		if l.Checksum, err = util.ReadByte(rd); err != nil {
			return err
		}
	}
	return nil
}

func (l *LastSeenMessages) Empty() bool {
	return l.Acknowledged.Empty()
}

var divFloor = -mathutil.FloorDiv(-20, 8)

// copyOf equivalent to Java's Arrays.copyOf(acknowledged.toByteArray(), DIV_FLOOR)
func copyOf(acknowledged mathutil.BitSet) []byte {
	bytes := make([]byte, divFloor)
	copy(bytes, acknowledged.Bytes)
	return bytes
}

package proto

import (
	"go.minekube.com/gate/pkg/edition/bedrock/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

// PacketMeta is a Bedrock edition packet.
type PacketMeta struct {
	Header Header
	proto.PacketContext
}

func init() {
	p := PacketMeta{}
	p.
}

// Header is the header of a packet containing
// a one varuint32 which is composed of a packet id,
// a sender and target sub client id.
// These ids are used for split screen functionality.
type Header struct {
	PacketID        uint32
	SenderSubClient byte
	TargetSubClient byte
}

func (h *Header) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteVarUint32(wr, h.PacketID|(uint32(h.SenderSubClient)<<10)|(uint32(h.TargetSubClient)<<12))
}

func (h *Header) Decode(_ *proto.PacketContext, rd io.Reader) error {
	value, err := util.ReadVarUint32(rd)
	if err != nil {
		return err
	}
	h.PacketID = value & 0x3FF
	h.SenderSubClient = byte((value >> 10) & 0x3)
	h.TargetSubClient = byte((value >> 12) & 0x3)
	return nil
}

var _ proto.Packet = (*Header)(nil)

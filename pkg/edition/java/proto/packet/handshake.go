package packet

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

// https://wiki.vg/Protocol#Handshaking
type Handshake struct {
	ProtocolVersion int
	ServerAddress   string
	Port            int16
	NextStatus      int
}

func (h *Handshake) Encode(_ *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, h.ProtocolVersion)
	if err != nil {
		return err
	}
	err = util.WriteString(wr, h.ServerAddress)
	if err != nil {
		return err
	}
	err = util.WriteInt16(wr, h.Port)
	if err != nil {
		return err
	}
	return util.WriteVarInt(wr, h.NextStatus)
}

func (h *Handshake) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	h.ProtocolVersion, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	h.ServerAddress, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	h.Port, err = util.ReadInt16(rd)
	if err != nil {
		return err
	}
	h.NextStatus, err = util.ReadVarInt(rd)
	return err
}

var _ proto.Packet = (*Handshake)(nil)

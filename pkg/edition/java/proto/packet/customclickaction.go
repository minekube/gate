package packet

import (
	"io"

	"go.minekube.com/gate/pkg/gate/proto"
)

// ServerboundCustomClickActionPacket is sent by the client when clicking on a custom action.
type ServerboundCustomClickActionPacket struct {
	Data []byte
}

var _ proto.Packet = (*ServerboundCustomClickActionPacket)(nil)

func (s *ServerboundCustomClickActionPacket) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	_, err := wr.Write(s.Data)
	return err
}

func (s *ServerboundCustomClickActionPacket) Decode(ctx *proto.PacketContext, rd io.Reader) error {
	// Read all remaining bytes
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	s.Data = data
	return nil
}

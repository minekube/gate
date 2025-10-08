package packet

import (
	"io"

	"go.minekube.com/gate/pkg/gate/proto"
)

// CustomClickActionPacket is sent by the client when clicking on a custom action.
type CustomClickActionPacket struct {
	Data []byte
}

var _ proto.Packet = (*CustomClickActionPacket)(nil)

func (s *CustomClickActionPacket) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	_, err := wr.Write(s.Data)
	return err
}

func (s *CustomClickActionPacket) Decode(ctx *proto.PacketContext, rd io.Reader) error {
	// Read all remaining bytes
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	s.Data = data
	return nil
}

package config

import (
	"io"

	"go.minekube.com/gate/pkg/gate/proto"
)

// CodeOfConductAcceptPacket is sent by the client to accept the code of conduct.
type CodeOfConductAcceptPacket struct{}

var _ proto.Packet = (*CodeOfConductAcceptPacket)(nil)

func (c *CodeOfConductAcceptPacket) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	// Empty packet
	return nil
}

func (c *CodeOfConductAcceptPacket) Decode(ctx *proto.PacketContext, rd io.Reader) error {
	// Empty packet
	return nil
}

// CodeOfConductPacket is sent by the server to display the code of conduct.
type CodeOfConductPacket struct {
	Data []byte
}

var _ proto.Packet = (*CodeOfConductPacket)(nil)

func (c *CodeOfConductPacket) Encode(ctx *proto.PacketContext, wr io.Writer) error {
	_, err := wr.Write(c.Data)
	return err
}

func (c *CodeOfConductPacket) Decode(ctx *proto.PacketContext, rd io.Reader) error {
	// Read all remaining bytes
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	c.Data = data
	return nil
}

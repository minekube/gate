package plugin

import (
	"io"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Message is a Minecraft plugin message packet.
type Message struct {
	Channel string
	Data    []byte
}

func (p *Message) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		err = util.WriteString(wr, TransformLegacyToModernChannel(p.Channel))
	} else {
		err = util.WriteString(wr, p.Channel)
	}
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		_, err = wr.Write(p.Data)
	} else {
		err = util.WriteBytes17(wr, p.Data, true) // true for Forge support
	}
	return
}

func (p *Message) Decode(c *proto.PacketContext, r io.Reader) (err error) {
	p.Channel, err = util.ReadString(r)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		p.Channel = TransformLegacyToModernChannel(p.Channel)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		p.Data, err = io.ReadAll(r)
	} else {
		p.Data, err = util.ReadBytes17(r)
	}
	return
}

var _ proto.Packet = (*Message)(nil)

package plugin

import (
	"bytes"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
	"io/ioutil"
)

// Message is a Minecraft plugin message packet.
type Message struct {
	Channel string
	Data    []byte

	// Not part of the packet!
	// This is to store the decoded packet bytes as is
	// to forward them without needing to encode the packet again.
	Retained []byte
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
		err = util.WriteBytes(wr, p.Data)
	} else {
		err = util.WriteBytes17(wr, p.Data, true) // true for Forge support
	}
	return
}

func (p *Message) Decode(c *proto.PacketContext, r io.Reader) (err error) {
	retained := new(bytes.Buffer)
	rd := io.TeeReader(r, retained)

	p.Channel, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_13) {
		p.Channel = TransformLegacyToModernChannel(p.Channel)
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		p.Data, err = ioutil.ReadAll(rd)
	} else {
		p.Data, err = util.ReadBytes17(rd)
	}

	p.Retained = retained.Bytes()
	return
}

var _ proto.Packet = (*Message)(nil)

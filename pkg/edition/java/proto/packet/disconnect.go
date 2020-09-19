package packet

import (
	"errors"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/runtime/logr"
	"io"
	"strings"
)

type Disconnect struct {
	Reason *string // A reason must only be given for encoding.
}

func (d *Disconnect) Encode(c *proto.PacketContext, wr io.Writer) error {
	if d.Reason == nil {
		return errors.New("missing reason for disconnect")
	}
	return util.WriteString(wr, *d.Reason)
}

func (d *Disconnect) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	s, err := util.ReadString(rd)
	if err != nil {
		return err
	}
	d.Reason = &s
	return nil
}

var _ proto.Packet = (*Disconnect)(nil)

// DisconnectWith creates a Disconnect packet with guaranteed reason.
func DisconnectWith(reason component.Component) *Disconnect {
	return DisconnectWithProtocol(reason, proto.Minecraft_1_7_2.Protocol)
}

var log = logr.Log.WithName("packet")

// DisconnectWithProtocol creates a new Disconnect packet for the given given protocol.
func DisconnectWithProtocol(reason component.Component, protocol proto.Protocol) *Disconnect {
	if reason == nil {
		reason = &component.Text{} // empty reason
	}
	b := new(strings.Builder)
	if err := util.JsonCodec(protocol).Marshal(b, reason); err != nil {
		log.V(1).WithName("DisconnectWithProtocol").Error(err,
			"Error marshal disconnect reason component to json",
			"reason", reason,
			"protocol", protocol)
		b.Reset() // empty reason
	}
	s := b.String()
	return &Disconnect{Reason: &s}
}

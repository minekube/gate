package packet

import (
	"errors"
	"io"
	"log/slog"

	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type Disconnect struct {
	Reason *chat.ComponentHolder // nil-able
}

func (d *Disconnect) Encode(c *proto.PacketContext, wr io.Writer) error {
	if d.Reason == nil {
		return errors.New("no reason specified")
	}
	if c.PacketID == 0x00 && c.Direction == proto.ClientBound { // states.LoginState
		c.Protocol = version.Minecraft_1_20_2.Protocol
	}
	return d.Reason.Write(wr, c.Protocol)
}

func (d *Disconnect) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	protocol := c.Protocol
	if c.PacketID == 0x00 && c.Direction == proto.ClientBound { // states.LoginState
		protocol = version.Minecraft_1_20_2.Protocol
	}
	d.Reason, err = chat.ReadComponentHolder(rd, protocol)
	return err
}

var _ proto.Packet = (*Disconnect)(nil)

// NewDisconnect creates a new Disconnect packet.
func NewDisconnect(reason component.Component, protocol proto.Protocol, stat states.State) *Disconnect {
	if stat == states.LoginState {
		protocol = version.Minecraft_1_20_2.Protocol
	}
	if reason == nil {
		slog.Error("tried to create a Disconnect packet with a nil reason")
		reason = &component.Text{Content: ""}
	}
	return &Disconnect{
		Reason: chat.FromComponentProtocol(reason, protocol),
	}
}

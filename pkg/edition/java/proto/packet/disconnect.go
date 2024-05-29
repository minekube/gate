package packet

import (
	"errors"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"io"
	"log/slog"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

type Disconnect struct {
	Reason *chat.ComponentHolder // nil-able

	// Not part of the packet data itself,
	// but used to determine the state of the client.
	State states.State
}

func (d *Disconnect) Encode(c *proto.PacketContext, wr io.Writer) error {
	if d.Reason == nil {
		return errors.New("no reason specified")
	}
	return d.Reason.Write(wr, c.Protocol)
}

func (d *Disconnect) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	protocol := c.Protocol
	if d.State == states.LoginState {
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
		State:  stat,
	}
}

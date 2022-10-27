// Package title contains title packets.
package title

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Action is the title action.
type Action int

// Title packets actions.
// (Numbered after the 1.11+ increment by 1)
const (
	SetTitle Action = iota
	SetSubtitle
	SetActionBar
	SetTimes
	Hide
	Reset
)

// ProtocolAction returns the correct action id for the protocol version
// since 1.11+ shifted the action enum by 1 to handle the action bar.
func ProtocolAction(protocol proto.Protocol, action Action) Action {
	if protocol.Lower(version.Minecraft_1_11) && action > 2 {
		return action - 1
	}
	return action
}

// New creates a version and type dependent title packet.
func New(protocol proto.Protocol, title *Builder) (titlePacket proto.Packet, err error) {
	var c string
	if title.Component != nil {
		c, err = comp(protocol, title.Component)
		if err != nil {
			return nil, err
		}
	}

	if protocol.GreaterEqual(version.Minecraft_1_17) {
		switch title.Action {
		case SetActionBar:
			return &Actionbar{Component: c}, nil
		case SetSubtitle:
			return &Subtitle{Component: c}, nil
		case SetTimes:
			return &Times{
				FadeIn:  title.FadeIn,
				Stay:    title.Stay,
				FadeOut: title.FadeOut,
			}, nil
		case SetTitle:
			return &Text{Component: c}, nil
		case Reset:
			return &Clear{Action: Reset}, nil
		case Hide:
			return &Clear{Action: Hide}, nil
		default:
			// Invalid action type, fallback to Reset
			return &Clear{Action: Reset}, nil
		}
	}

	return &Legacy{
		Action:    title.Action,
		Component: c,
		FadeIn:    title.FadeIn,
		Stay:      title.Stay,
		FadeOut:   title.FadeOut,
	}, err
}

func comp(protocol proto.Protocol, c component.Component) (string, error) {
	if c == nil {
		return "", errors.New("component must not be nil")
	}
	b := new(strings.Builder)
	err := util.JsonCodec(protocol).Marshal(b, c)
	return b.String(), err
}

// Builder is a Title packet builder.
type Builder struct {
	Action                Action
	Component             component.Component
	FadeIn, Stay, FadeOut int // ticks
}

type (
	Actionbar struct{ Component string }
	Subtitle  struct{ Component string }
	Times     struct{ FadeIn, Stay, FadeOut int }
	Text      struct{ Component string }
	Clear     struct {
		// Either Hide or Reset. Falls back to Hide.
		Action Action
	}
)

func (t *Text) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteString(wr, t.Component)
}
func (t *Text) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	t.Component, err = util.ReadString(rd)
	return
}
func (c *Clear) Decode(_ *proto.PacketContext, rd io.Reader) error {
	a, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if a {
		c.Action = Reset
	} else {
		c.Action = Hide
	}
	return nil
}
func (c *Clear) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteBool(wr, c.Action == Reset)
}
func (a *Actionbar) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	a.Component, err = util.ReadString(rd)
	return
}
func (a *Actionbar) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteString(wr, a.Component)
}
func (s *Subtitle) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	s.Component, err = util.ReadString(rd)
	return
}
func (s *Subtitle) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteString(wr, s.Component)
}
func (t *Times) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	t.FadeIn, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	t.Stay, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	t.FadeOut, err = util.ReadInt(rd)
	return
}
func (t *Times) Encode(_ *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt(wr, t.FadeIn)
	if err != nil {
		return err
	}
	err = util.WriteInt(wr, t.Stay)
	if err != nil {
		return err
	}
	return util.WriteInt(wr, t.FadeOut)
}

type Legacy struct {
	Action                Action
	Component             string
	FadeIn, Stay, FadeOut int
}

func (l *Legacy) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.Lower(version.Minecraft_1_11) && l.Action == SetActionBar {
		return errors.New("action bars are only supported on 1.11+")
	}
	err := util.WriteVarInt(wr, int(ProtocolAction(c.Protocol, l.Action)))
	if err != nil {
		return err
	}

	switch l.Action {
	default:
		return fmt.Errorf("unknown action %d", l.Action)
	case Hide, Reset:
		return nil
	case SetTitle, SetSubtitle, SetActionBar:
		return util.WriteString(wr, l.Component)
	case SetTimes:
		err = util.WriteInt(wr, l.FadeIn)
		if err != nil {
			return err
		}
		err = util.WriteInt(wr, l.Stay)
		if err != nil {
			return err
		}
		return util.WriteInt(wr, l.FadeOut)
	}
}
func (l *Legacy) Decode(c *proto.PacketContext, rd io.Reader) error {
	action, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	if c.Protocol.Lower(version.Minecraft_1_11) && action > 1 {
		// 1.11+ shifted the action enum by 1 to handle the action bar
		action += 1
	}
	l.Action = Action(action)

	switch l.Action {
	default:
		return fmt.Errorf("unknown action %d", l.Action)
	case Hide, Reset:
		return nil
	case SetTitle, SetSubtitle, SetActionBar:
		l.Component, err = util.ReadString(rd)
	case SetTimes:
		l.FadeIn, err = util.ReadInt(rd)
		if err != nil {
			return err
		}
		l.Stay, err = util.ReadInt(rd)
		if err != nil {
			return err
		}
		l.FadeOut, err = util.ReadInt(rd)
	}
	return err
}

var _ proto.Packet = (*Legacy)(nil)
var _ proto.Packet = (*Actionbar)(nil)
var _ proto.Packet = (*Subtitle)(nil)
var _ proto.Packet = (*Times)(nil)
var _ proto.Packet = (*Text)(nil)
var _ proto.Packet = (*Clear)(nil)

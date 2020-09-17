package packet

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"io"
)

// Signifies that a packet is only meant to be encoded, no decoding.
var ErrEncodeOnly = errors.New("packet is only to be encoded")

type TitleAction int

// Title packet actions
const (
	SetTitle     TitleAction = 0
	SetSubtitle  TitleAction = 1
	SetActionBar TitleAction = 2
	SetTimes     TitleAction = 3
	Hide         TitleAction = 4
	Reset        TitleAction = 5

	// 1.11+ shifted the action enum by 1 to handle the action bar
	SetTimesOld TitleAction = 2
	HideOld     TitleAction = 3
	ResetOld    TitleAction = 4
)

type Title struct {
	Action                TitleAction
	Component             *string // nil-able
	FadeIn, Stay, FadeOut int
}

func (t *Title) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, int(t.Action))
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(proto.Minecraft_1_11) {
		// 1.11+ shifted the action enum by 1 to handle the action bar
		switch t.Action {
		case SetTitle, SetSubtitle, Hide, Reset:
		case SetActionBar:
			if t.Component == nil {
				return fmt.Errorf("no component found for action %d", t.Action)
			}
			err = util.WriteString(wr, *t.Component)
			if err != nil {
				return err
			}
		case SetTimes:
			err = util.WriteInt32(wr, int32(t.FadeIn))
			if err != nil {
				return err
			}
			err = util.WriteInt32(wr, int32(t.Stay))
			if err != nil {
				return err
			}
			err = util.WriteInt32(wr, int32(t.FadeOut))
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown action %d", t.Action)
		}
	} else {
		switch t.Action {
		case SetTitle, HideOld, ResetOld:
		case SetSubtitle:
			if t.Component == nil {
				return fmt.Errorf("no component found for action %d", t.Action)
			}
			err = util.WriteString(wr, *t.Component)
			if err != nil {
				return err
			}
		case SetTimesOld:
			err = util.WriteInt32(wr, int32(t.FadeIn))
			if err != nil {
				return err
			}
			err = util.WriteInt32(wr, int32(t.Stay))
			if err != nil {
				return err
			}
			err = util.WriteInt32(wr, int32(t.FadeOut))
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown action %d", t.Action)
		}
	}
	return nil
}

func NewHideTitle(protocol proto.Protocol) *Title {
	return &Title{Action: HideTitleAction(protocol)}
}
func NewResetTitle(protocol proto.Protocol) *Title {
	return &Title{Action: ResetTitleAction(protocol)}
}
func HideTitleAction(protocol proto.Protocol) TitleAction {
	if protocol.GreaterEqual(proto.Minecraft_1_11) {
		return Hide
	}
	return HideOld
}
func ResetTitleAction(protocol proto.Protocol) TitleAction {
	if protocol.GreaterEqual(proto.Minecraft_1_11) {
		return Reset
	}
	return ResetOld
}
func TimesTitleAction(protocol proto.Protocol) TitleAction {
	if protocol.GreaterEqual(proto.Minecraft_1_11) {
		return SetTimes
	}
	return SetTimesOld
}

func (t *Title) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	return ErrEncodeOnly
}

var _ proto.Packet = (*Title)(nil)

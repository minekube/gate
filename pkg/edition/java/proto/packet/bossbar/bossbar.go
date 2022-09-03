package bossbar

import (
	"errors"
	"io"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type Action int

const (
	AddAction Action = iota
	RemoveAction
	UpdatePercentAction
	UpdateNameAction
	UpdateStyleAction
	UpdatePropertiesAction
)

type Color int

const (
	PinkColor Color = iota
	BlueColor
	RedColor
	GreenColor
	YellowColor
	PurpleColor
	WhiteColor
)

type Overlay int

const (
	ProgressOverlay Overlay = iota
	Notched6Overlay
	Notched10Overlay
	Notched12Overlay
	Notched20Overlay
)

type Flag int

const (
	DarkenScreenFlag   Flag = 0x01
	PlayBossMusicFlag  Flag = 0x02
	CreateWorldFogFlag Flag = 0x04
)

var (
	errNoName        = errors.New("action bar needs to have a name specified")
	errInvalidAction = errors.New("unknown action for boss bar")
)

type BossBar struct {
	ID      uuid.UUID
	Action  Action
	Name    component.Component
	Percent float32
	Color   Color
	Overlay Overlay
	Flags   byte
}

func (bb *BossBar) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteUUID(wr, bb.ID)
	if err != nil {
		return err
	}
	err = util.WriteVarInt(wr, int(bb.Action))
	if err != nil {
		return err
	}

	switch bb.Action {
	case AddAction:
		if bb.Name == nil {
			return errNoName
		}
		err = util.WriteComponent(wr, c.Protocol, bb.Name)
		if err != nil {
			return err
		}
		err = util.WriteFloat32(wr, bb.Percent)
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, int(bb.Color))
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, int(bb.Overlay))
		if err != nil {
			return err
		}
		err = util.WriteByte(wr, bb.Flags)
		if err != nil {
			return err
		}
	case RemoveAction:
		// do nohing
	case UpdatePercentAction:
		err = util.WriteFloat32(wr, bb.Percent)
		if err != nil {
			return err
		}
	case UpdateNameAction:
		if bb.Name == nil {
			return errNoName
		}
		err = util.WriteComponent(wr, c.Protocol, bb.Name)
		if err != nil {
			return err
		}
	case UpdateStyleAction:
		err = util.WriteVarInt(wr, int(bb.Color))
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, int(bb.Overlay))
		if err != nil {
			return err
		}
	case UpdatePropertiesAction:
		err = util.WriteByte(wr, bb.Flags)
		if err != nil {
			return err
		}
	default:
		return errInvalidAction
	}
	return nil
}

func (bb *BossBar) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	bb.ID, err = util.ReadUUID(rd)
	if err != nil {
		return err
	}
	tmpAction, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	bb.Action = Action(tmpAction)

	switch bb.Action {
	case AddAction:
		bb.Name, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
		bb.Percent, err = util.ReadFloat32(rd)
		if err != nil {
			return err
		}
		tmpColor, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Color = Color(tmpColor)
		tmpOverlay, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Overlay = Overlay(tmpOverlay)
		bb.Flags, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
	case RemoveAction:
		// do nothing
	case UpdatePercentAction:
		bb.Percent, err = util.ReadFloat32(rd)
		if err != nil {
			return err
		}
	case UpdateNameAction:
		bb.Name, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	case UpdateStyleAction:
		tmpColor, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Color = Color(tmpColor)
		tmpOverlay, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Overlay = Overlay(tmpOverlay)
	case UpdatePropertiesAction:
		bb.Flags, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
	default:
		return errInvalidAction
	}
	return nil
}

// ConvertFlags converts the given flags to the byte representation.
func ConvertFlags(flags ...Flag) byte {
	var val byte
	for _, flag := range flags {
		val |= byte(flag)
	}
	return val
}

package packet

import (
	"errors"
	"io"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type BossBarAction uint8

const (
	BossBarActionAdd BossBarAction = iota
	BossBarActionRemove
	BossBarActionUpdatePercent
	BossBarActionUpdateName
	BossBarActionUpdateStyle
	BossBarActionProperties
)

type BossBarColor uint8

const (
	BossBarColorPink BossBarColor = iota
	BossBarColorBlue
	BossBarColorRed
	BossBarColorGreen
	BossBarColorYellow
	BossBarColorPurple
	BossBarColorWhite
)

type BossBarOverlay uint8

const (
	BossBarOverlayProgress BossBarOverlay = iota
	BossBarOverlayNotched6
	BossBarOverlayNotched10
	BossBarOverlayNotched12
	BossBarOverlayNotched20
)

type BossBarFlag uint8

const (
	BossBarFlagDarkenScreen   BossBarFlag = 0x01
	BossBarFlagPlayBossMusic  BossBarFlag = 0x02
	BossBarFlagCreateWorldFog BossBarFlag = 0x04
)

var (
	errBossBarNoName        = errors.New("action bar needs to have a name specified")
	errBossBarInvalidAction = errors.New("unknown action for bossbar")
)

type BossBar struct {
	UUID    uuid.UUID
	Action  BossBarAction
	Name    component.Component
	Percent float32
	Color   BossBarColor
	Overlay BossBarOverlay
	Flags   byte
}

func (bb *BossBar) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteUUID(wr, bb.UUID)
	if err != nil {
		return err
	}
	err = util.WriteVarInt(wr, int(bb.Action))
	if err != nil {
		return err
	}

	switch bb.Action {
	case BossBarActionAdd:
		if bb.Name == nil {
			return errBossBarNoName
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
	case BossBarActionRemove:
		// do nohing
	case BossBarActionUpdatePercent:
		if bb.Name == nil {
			return errBossBarNoName
		}
		err = util.WriteComponent(wr, c.Protocol, bb.Name)
		if err != nil {
			return err
		}
	case BossBarActionUpdateStyle:
		err = util.WriteVarInt(wr, int(bb.Color))
		if err != nil {
			return err
		}
		err = util.WriteVarInt(wr, int(bb.Overlay))
		if err != nil {
			return err
		}
	case BossBarActionProperties:
		err = util.WriteByte(wr, bb.Flags)
		if err != nil {
			return err
		}
	default:
		return errBossBarInvalidAction
	}

	return nil
}

func (bb *BossBar) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	bb.UUID, err = util.ReadUUID(rd)
	if err != nil {
		return err
	}
	tmpAction, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	bb.Action = BossBarAction(tmpAction)

	switch bb.Action {
	case BossBarActionAdd:
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
		bb.Color = BossBarColor(tmpColor)
		tmpOverlay, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Overlay = BossBarOverlay(tmpOverlay)
		bb.Flags, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
	case BossBarActionRemove:
		// do nothing
	case BossBarActionUpdatePercent:
		bb.Percent, err = util.ReadFloat32(rd)
		if err != nil {
			return err
		}
	case BossBarActionUpdateName:
		bb.Name, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	case BossBarActionUpdateStyle:
		tmpColor, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Color = BossBarColor(tmpColor)
		tmpOverlay, err := util.ReadVarInt(rd)
		if err != nil {
			return err
		}
		bb.Overlay = BossBarOverlay(tmpOverlay)
	case BossBarActionProperties:
		bb.Flags, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
	default:
		return errBossBarInvalidAction
	}
	return nil
}

func BossBarFlags(flags []BossBarFlag) byte {
	var val byte
	for _, flag := range flags {
		val |= byte(flag)
	}

	return val
}

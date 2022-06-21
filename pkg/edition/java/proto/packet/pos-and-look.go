package packet

import (
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"io"
)

// PlayerPosAndLook 0x38 sets player position and look direction.
// dismisses the "loading terrain" when connecting.
// pos is absolute or relative depending on the flags
type PlayerPosAndLook struct {
	X     float64
	Y     float64
	Z     float64
	Yaw   float32
	Pitch float32

	// Flags is a bit field, for controlling if value is relative
	// X/Y/Z/Y_ROT/X_ROT
	// X: 0x01, Y: 0x02, Z: 0x04, Y_ROT 0x08, X_ROT 0x10
	Flags byte

	// TeleportID (varint) is the teleport identifier
	// If set, should follow this up with a "TeleportConfirm"
	TeleportID int
	// DismountVehicle indicates player should dismount their vehicle
	DismountVehicle bool
}

func (r *PlayerPosAndLook) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if err := util.WriteFloat64(wr, r.X); err != nil {
		return err
	}
	if err := util.WriteFloat64(wr, r.Y); err != nil {
		return err
	}
	if err := util.WriteFloat64(wr, r.Z); err != nil {
		return err
	}
	if err := util.WriteFloat32(wr, r.Yaw); err != nil {
		return err
	}
	if err := util.WriteFloat32(wr, r.Pitch); err != nil {
		return err
	}
	if err := util.WriteByte(wr, r.Flags); err != nil {
		return err
	}
	if err := util.WriteVarInt(wr, r.TeleportID); err != nil {
		return err
	}
	if err := util.WriteBool(wr, r.DismountVehicle); err != nil {
		return err
	}
	return nil
}

func (r *PlayerPosAndLook) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	r.X, err = util.ReadFloat64(rd)
	if err != nil {
		return err
	}
	r.Y, err = util.ReadFloat64(rd)
	if err != nil {
		return err
	}
	r.Z, err = util.ReadFloat64(rd)
	if err != nil {
		return err
	}
	r.Yaw, err = util.ReadFloat32(rd)
	if err != nil {
		return err
	}
	r.Pitch, err = util.ReadFloat32(rd)
	if err != nil {
		return err
	}
	r.Flags, err = util.ReadByte(rd)
	if err != nil {
		return err
	}
	r.TeleportID, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	r.DismountVehicle, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	return nil
}

var _ proto.Packet = (*PlayerPosAndLook)(nil)

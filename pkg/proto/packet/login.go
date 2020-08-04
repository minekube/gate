package packet

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/util"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/uuid"
	"io"
)

type ServerLogin struct {
	Username string
}

var errEmptyUsername = errs.NewSilentErr("empty username")

const maxUsernameLen = 16

func (s *ServerLogin) Encode(_ *proto.PacketContext, wr io.Writer) error {
	return util.WriteString(wr, s.Username)
}

func (s *ServerLogin) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	s.Username, err = util.ReadStringMax(rd, maxUsernameLen)
	if len(s.Username) == 0 {
		return errEmptyUsername
	}
	return
}

type EncryptionResponse struct {
	SharedSecret  []byte
	VerifiedToken []byte
}

func (e *EncryptionResponse) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(proto.Minecraft_1_8) {
		err := util.WriteBytes(wr, e.SharedSecret)
		if err != nil {
			return err
		}
		return util.WriteBytes(wr, e.VerifiedToken)
	} else {
		err := util.WriteBytes17(wr, e.SharedSecret, false)
		if err != nil {
			return err
		}
		return util.WriteBytes17(wr, e.VerifiedToken, false)
	}
}

func (e *EncryptionResponse) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(proto.Minecraft_1_8) {
		e.SharedSecret, err = util.ReadBytesLen(rd, 256)
		if err != nil {
			return
		}
		e.VerifiedToken, err = util.ReadBytesLen(rd, 128)
	} else {
		e.SharedSecret, err = util.ReadBytes17(rd)
		if err != nil {
			return
		}
		e.VerifiedToken, err = util.ReadBytes17(rd)
	}
	return
}

type LoginPluginResponse struct {
	Id      int
	Success bool
	Data    []byte
}

func (l *LoginPluginResponse) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	err = util.WriteVarInt(wr, l.Id)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, l.Success)
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, l.Data)
}

func (l *LoginPluginResponse) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	l.Id, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	l.Success, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	l.Data, err = util.ReadBytes(rd)
	if errors.Is(err, io.EOF) { // TODO is this the check for whether rd is readable?
		return nil
	}
	return
}

type EncryptionRequest struct {
	ServerId    string
	PublicKey   []byte
	VerifyToken []byte
}

func (e *EncryptionRequest) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, e.ServerId)
	if err != nil {
		return err
	}
	err = util.WriteBytes(wr, e.PublicKey)
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, e.VerifyToken)
}

func (e *EncryptionRequest) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	e.ServerId, err = util.ReadStringMax(rd, 20)
	if err != nil {
		return err
	}
	e.PublicKey, err = util.ReadBytesLen(rd, 256)
	if err != nil {
		return err
	}
	e.VerifyToken, err = util.ReadBytesLen(rd, 16)
	return err
}

type ServerLoginSuccess struct {
	UUID     uuid.UUID
	Username string
}

func (s *ServerLoginSuccess) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		err = util.WriteUuid(wr, s.UUID)
	} else if c.Protocol.GreaterEqual(proto.Minecraft_1_7_6) {
		err = util.WriteString(wr, s.UUID.String())
	} else {
		err = util.WriteString(wr, s.UUID.Undashed())
	}
	if err != nil {
		return err
	}
	return util.WriteString(wr, s.Username)
}

func (s *ServerLoginSuccess) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	var uuidString string
	if c.Protocol.GreaterEqual(proto.Minecraft_1_16) {
		s.UUID, err = util.ReadUuid(rd)
	} else {
		if c.Protocol.GreaterEqual(proto.Minecraft_1_7_6) {
			uuidString, err = util.ReadStringMax(rd, 36)
		} else {
			uuidString, err = util.ReadStringMax(rd, 32)
		}
		if err != nil {
			return
		}
		s.UUID, err = uuid.Parse(uuidString)
		if err != nil {
			return fmt.Errorf("error parsing uuid: %v", err)
		}
	}
	if err != nil {
		return
	}
	s.Username, err = util.ReadStringMax(rd, maxUsernameLen)
	return
}

type SetCompression struct {
	Threshold int
}

func (s *SetCompression) Encode(c *proto.PacketContext, wr io.Writer) error {
	return util.WriteVarInt(wr, s.Threshold)
}

func (s *SetCompression) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	s.Threshold, err = util.ReadVarInt(rd)
	return
}

type LoginPluginMessage struct {
	Id      int
	Channel string
	Data    []byte
}

func (l *LoginPluginMessage) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, l.Id)
	if err != nil {
		return err
	}
	err = util.WriteString(wr, l.Channel)
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, l.Data)
}

func (l *LoginPluginMessage) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	l.Id, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	l.Channel, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	l.Data, err = util.ReadBytes(rd)
	if errors.Is(err, io.EOF) { // TODO is this the check for whether rd is readable?
		return nil
	}
	return
}

var _ proto.Packet = (*ServerLogin)(nil)
var _ proto.Packet = (*ServerLoginSuccess)(nil)
var _ proto.Packet = (*LoginPluginMessage)(nil)
var _ proto.Packet = (*LoginPluginResponse)(nil)
var _ proto.Packet = (*EncryptionRequest)(nil)
var _ proto.Packet = (*EncryptionResponse)(nil)
var _ proto.Packet = (*SetCompression)(nil)

package packet

import (
	"errors"
	"fmt"
	"io"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/uuid"
)

type ServerLogin struct {
	Username  string
	PlayerKey crypto.IdentifiedKey // 1.19.3
	HolderID  uuid.UUID            // Used for key revision 2
}

var errEmptyUsername = errs.NewSilentErr("empty username")

const maxUsernameLen = 16

func (s *ServerLogin) Encode(c *proto.PacketContext, wr io.Writer) error {
	if s.Username == "" {
		return errors.New("username not specified")
	}
	err := util.WriteString(wr, s.Username)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		if c.Protocol.Lower(version.Minecraft_1_19_3) {
			err = util.WriteBool(wr, s.PlayerKey != nil)
			if err != nil {
				return err
			}
			if s.PlayerKey != nil {
				err = crypto.WritePlayerKey(wr, s.PlayerKey)
				if err != nil {
					return err
				}
			}
		}

		if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
			ok := (s.PlayerKey != nil && s.PlayerKey.SignatureHolder() != uuid.Nil) || s.HolderID != uuid.Nil
			err = util.WriteBool(wr, ok)
			if err != nil {
				return err
			}
			if ok {
				id := s.PlayerKey.SignatureHolder()
				if s.HolderID != uuid.Nil {
					id = s.HolderID
				}
				err = util.WriteUUID(wr, id)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *ServerLogin) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	s.Username, err = util.ReadStringMax(rd, maxUsernameLen)
	if len(s.Username) == 0 {
		return errEmptyUsername
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		if c.Protocol.GreaterEqual(version.Minecraft_1_19_3) {
			s.PlayerKey = nil
		} else {
			ok, err := util.ReadBool(rd)
			if err != nil {
				return err
			}
			if ok {
				s.PlayerKey, err = crypto.ReadPlayerKey(c.Protocol, rd)
				if err != nil {
					return err
				}
			} else {
				s.PlayerKey = nil
			}
		}

		if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
			ok, err := util.ReadBool(rd)
			if err != nil {
				return err
			}
			if ok {
				s.HolderID, err = util.ReadUUID(rd)
				if err != nil {
					return err
				}
			}
		}
	} else {
		s.PlayerKey = nil
	}
	return
}

type EncryptionResponse struct {
	SharedSecret []byte
	VerifyToken  []byte
	Salt         *int64 // 1.19+
}

func (e *EncryptionResponse) Encode(c *proto.PacketContext, wr io.Writer) error {
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		err := util.WriteBytes(wr, e.SharedSecret)
		if err != nil {
			return err
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_19) && c.Protocol.Lower(version.Minecraft_1_19_3) {
			err = util.WriteBool(wr, e.Salt == nil) // yes, write true if no salt
			if err != nil {
				return err
			}
			if e.Salt != nil {
				err = util.WriteInt64(wr, *e.Salt)
				if err != nil {
					return err
				}
			}
		}
		return util.WriteBytes(wr, e.VerifyToken)
	} else {
		err := util.WriteBytes17(wr, e.SharedSecret, false)
		if err != nil {
			return err
		}
		return util.WriteBytes17(wr, e.VerifyToken, false)
	}
}

func (e *EncryptionResponse) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		e.SharedSecret, err = util.ReadBytesLen(rd, 128)
		if err != nil {
			return
		}

		if c.Protocol.GreaterEqual(version.Minecraft_1_19) && c.Protocol.Lower(version.Minecraft_1_19_3) {
			var ok bool
			ok, err = util.ReadBool(rd)
			if err != nil {
				return err
			}
			if !ok { // yes, bool must be false
				salt, err := util.ReadInt64(rd)
				if err != nil {
					return err
				}
				e.Salt = &salt
			}
		}

		limit := 256
		if c.Protocol.Lower(version.Minecraft_1_19) {
			limit = 128
		}
		e.VerifyToken, err = util.ReadBytesLen(rd, limit)
		if err != nil {
			return
		}

	} else {
		e.SharedSecret, err = util.ReadBytes17(rd)
		if err != nil {
			return
		}
		e.VerifyToken, err = util.ReadBytes17(rd)
	}
	return
}

type LoginPluginResponse struct {
	ID      int
	Success bool
	Data    []byte
}

func (l *LoginPluginResponse) Encode(_ *proto.PacketContext, wr io.Writer) (err error) {
	err = util.WriteVarInt(wr, l.ID)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, l.Success)
	if err != nil {
		return err
	}
	return util.WriteRawBytes(wr, l.Data)
}

func (l *LoginPluginResponse) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	l.ID, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	l.Success, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	l.Data, err = util.ReadRawBytes(rd)
	if errors.Is(err, io.EOF) {
		// Ignore if we couldn't read data
		return nil
	}
	return
}

type EncryptionRequest struct {
	ServerID    string
	PublicKey   []byte
	VerifyToken []byte
}

func (e *EncryptionRequest) Encode(_ *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, e.ServerID)
	if err != nil {
		return err
	}
	err = util.WriteBytes(wr, e.PublicKey)
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, e.VerifyToken)
}

func (e *EncryptionRequest) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	e.ServerID, err = util.ReadStringMax(rd, 20)
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
	UUID       uuid.UUID
	Username   string
	Properties []profile.Property // 1.19+
}

func (s *ServerLoginSuccess) Encode(c *proto.PacketContext, wr io.Writer) (err error) {
	if s.Username == "" {
		return fmt.Errorf("no username specified")
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		err = util.WriteUUID(wr, s.UUID)
	} else if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		err = util.WriteUUID(wr, s.UUID)
	} else if c.Protocol.GreaterEqual(version.Minecraft_1_7_6) {
		err = util.WriteString(wr, s.UUID.String())
	} else {
		err = util.WriteString(wr, s.UUID.Undashed())
	}
	if err != nil {
		return err
	}
	err = util.WriteString(wr, s.Username)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		err = util.WriteProperties(wr, s.Properties)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ServerLoginSuccess) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		s.UUID, err = util.ReadUUID(rd)
	} else if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
		s.UUID, err = util.ReadUUID(rd) // readUUIDIntArray?
	} else {
		var uuidString string
		if c.Protocol.GreaterEqual(version.Minecraft_1_7_6) {
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
	if err != nil {
		return
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19) {
		s.Properties, err = util.ReadProperties(rd)
		if err != nil {
			return
		}
	}
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
	ID      int
	Channel string
	Data    []byte
}

func (l *LoginPluginMessage) Encode(_ *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, l.ID)
	if err != nil {
		return err
	}
	err = util.WriteString(wr, l.Channel)
	if err != nil {
		return err
	}
	return util.WriteBytes(wr, l.Data)
}

func (l *LoginPluginMessage) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	l.ID, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	l.Channel, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	l.Data, err = util.ReadBytes(rd)
	if errors.Is(err, io.EOF) {
		// Ignore if we couldn't read data
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

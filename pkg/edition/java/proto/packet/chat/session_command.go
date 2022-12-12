package chat

import (
	"io"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

type SessionPlayerCommand struct {
	Command            string
	Timestamp          time.Time
	Salt               int64
	ArgumentSignatures ArgumentSignatures
	LastSeenMessages   LastSeenMessages
}

var _ proto.Packet = (*SessionPlayerCommand)(nil)
var _ proto.Packet = (*ArgumentSignatures)(nil)
var _ proto.Packet = (*ArgumentSignature)(nil)

func (s *SessionPlayerCommand) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, s.Command)
	if err != nil {
		return err
	}
	err = util.WriteInt64(wr, s.Timestamp.UnixMilli())
	if err != nil {
		return err
	}
	err = util.WriteInt64(wr, s.Salt)
	if err != nil {
		return err
	}
	err = s.ArgumentSignatures.Encode(c, wr)
	if err != nil {
		return err
	}
	return s.LastSeenMessages.Encode(c, wr)
}

func (s *SessionPlayerCommand) Decode(c *proto.PacketContext, rd io.Reader) error {
	var err error
	s.Command, err = util.ReadStringMax(rd, 256)
	if err != nil {
		return err
	}
	s.Timestamp, err = util.ReadUnixMilli(rd)
	if err != nil {
		return err
	}
	s.Salt, err = util.ReadInt64(rd)
	if err != nil {
		return err
	}
	err = s.ArgumentSignatures.Decode(c, rd)
	if err != nil {
		return err
	}
	return s.LastSeenMessages.Decode(c, rd)
}

type ArgumentSignatures struct {
	Entries []ArgumentSignature
}

func (a *ArgumentSignatures) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteVarInt(wr, len(a.Entries))
	if err != nil {
		return err
	}
	for i := range a.Entries {
		err = a.Entries[i].Encode(c, wr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *ArgumentSignatures) Decode(c *proto.PacketContext, rd io.Reader) error {
	length, err := util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	const limit = 8
	if length > limit {
		return errs.NewSilentErr("too many argument signatures, %d is above limit of %d", length, limit)
	}
	a.Entries = make([]ArgumentSignature, length)
	for i := range a.Entries {
		err = a.Entries[i].Decode(c, rd)
		if err != nil {
			return err
		}
	}
	return nil
}

type ArgumentSignature struct {
	Name      string
	Signature []byte
}

func (a *ArgumentSignature) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, a.Name)
	if err != nil {
		return err
	}
	_, err = wr.Write(a.Signature)
	return err
}

func (a *ArgumentSignature) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	a.Name, err = util.ReadStringMax(rd, 16)
	if err != nil {
		return err
	}
	a.Signature, err = readMessageSignature(rd)
	return err
}

func readMessageSignature(rd io.Reader) ([]byte, error) {
	signature := make([]byte, 256)
	_, err := io.ReadFull(rd, signature)
	return signature, err
}

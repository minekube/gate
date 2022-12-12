package chat

import (
	"io"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

type SessionPlayerChat struct {
	Message          string
	Timestamp        time.Time
	Salt             int64
	Signed           bool
	Signature        []byte
	LastSeenMessages LastSeenMessages
}

var _ proto.Packet = (*SessionPlayerChat)(nil)

func (p *SessionPlayerChat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, p.Message)
	if err != nil {
		return err
	}
	err = util.WriteInt64(wr, p.Timestamp.UnixMilli())
	if err != nil {
		return err
	}
	err = util.WriteInt64(wr, p.Salt)
	if err != nil {
		return err
	}
	if p.Signed {
		err = util.WriteBytes(wr, p.Signature)
		if err != nil {
			return err
		}
	}
	return p.LastSeenMessages.Encode(c, wr)
}

func (p *SessionPlayerChat) Decode(c *proto.PacketContext, rd io.Reader) error {
	var err error
	p.Message, err = util.ReadStringMax(rd, 256)
	if err != nil {
		return err
	}
	p.Timestamp, err = util.ReadUnixMilli(rd)
	if err != nil {
		return err
	}
	p.Salt, err = util.ReadInt64(rd)
	if err != nil {
		return err
	}
	p.Signed, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	if p.Signed {
		p.Signature, err = util.ReadBytes(rd)
		if err != nil {
			return err
		}
	} else {
		p.Signature = nil
	}
	return p.LastSeenMessages.Decode(c, rd)
}

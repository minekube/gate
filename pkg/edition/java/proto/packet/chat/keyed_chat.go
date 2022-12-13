package chat

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
)

type KeyedPlayerChat struct {
	Message          string
	SignedPreview    bool
	Unsigned         bool
	Expiry           time.Time // may be zero if no salt or signature specified
	Signature        []byte
	Salt             []byte
	PreviousMessages []*crypto.SignaturePair
	LastMessage      *crypto.SignaturePair
}

const MaxPreviousMessageCount = 5

var errInvalidPreviousMessages = errs.NewSilentErr("invalid previous messages")

var (
	errInvalidSignature        = errs.NewSilentErr("incorrectly signed chat message")
	errPreviewSignatureMissing = errs.NewSilentErr("unsigned chat message requested signed preview")
)

func (p *KeyedPlayerChat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, p.Message)
	if err != nil {
		return err
	}

	if p.Unsigned {
		err = util.WriteInt64(wr, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		err = util.WriteInt64(wr, 0)
		if err != nil {
			return err
		}
		err = util.WriteBytes(wr, []byte{})
		if err != nil {
			return err
		}
	} else {
		err = util.WriteInt64(wr, p.Expiry.UnixMilli())
		if err != nil {
			return err
		}
		salt, _ := util.ReadInt64(bytes.NewReader(p.Salt))
		err = util.WriteInt64(wr, salt)
		if err != nil {
			return err
		}
		err = util.WriteBytes(wr, p.Signature)
		if err != nil {
			return err
		}
	}

	err = util.WriteBool(wr, p.SignedPreview)
	if err != nil {
		return err
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		err = encodePreviousAndLastMessages(c, wr, p.PreviousMessages, p.LastMessage)
		if err != nil {
			return err
		}
	}

	return nil
}

func encodePreviousAndLastMessages(
	c *proto.PacketContext,
	wr io.Writer,
	previousMessages []*crypto.SignaturePair,
	lastMessage *crypto.SignaturePair,
) error {
	err := util.WriteVarInt(wr, len(previousMessages))
	if err != nil {
		return err
	}
	for _, pm := range previousMessages {
		err = pm.Encode(c, wr)
		if err != nil {
			return err
		}
	}

	err = util.WriteBool(wr, lastMessage != nil)
	if err != nil {
		return err
	}
	if lastMessage == nil {
		return nil
	}
	return lastMessage.Encode(c, wr)
}

func (p *KeyedPlayerChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.Message, err = util.ReadStringMax(rd, MaxServerBoundMessageLength)
	if err != nil {
		return err
	}

	expiry, err := util.ReadInt64(rd)
	if err != nil {
		return err
	}
	salt, err := util.ReadInt64(rd)
	if err != nil {
		return err
	}
	signature, err := util.ReadBytes(rd)
	if err != nil {
		return err
	}

	if salt != 0 && len(signature) != 0 {
		buf := new(bytes.Buffer)
		_ = util.WriteInt64(buf, salt)
		p.Salt = buf.Bytes()
		p.Signature = signature
		p.Expiry = time.UnixMilli(expiry)
	} else if (c.Protocol.GreaterEqual(version.Minecraft_1_19_1) || salt == 0) && len(signature) == 0 {
		p.Unsigned = true
	} else {
		return errInvalidSignature
	}

	p.SignedPreview, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	if p.SignedPreview && p.Unsigned {
		return errPreviewSignatureMissing
	}

	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		p.PreviousMessages, p.LastMessage, err = decodePreviousAndLastMessages(c, rd)
		if err != nil {
			return err
		}
	}
	return nil
}

func decodePreviousAndLastMessages(c *proto.PacketContext, rd io.Reader) (
	previousMessages []*crypto.SignaturePair,
	lastMessage *crypto.SignaturePair,
	err error,
) {
	size, err := util.ReadVarInt(rd)
	if err != nil {
		return nil, nil, err
	}
	if size < 0 || size > MaxPreviousMessageCount {
		return nil, nil, fmt.Errorf("%w: max is %d but was %d",
			errInvalidPreviousMessages, MaxServerBoundMessageLength, size)
	}

	lastSignatures := make([]*crypto.SignaturePair, size)
	for i := 0; i < size; i++ {
		pair := new(crypto.SignaturePair)
		if err = pair.Decode(c, rd); err != nil {
			return nil, nil, err
		}
		lastSignatures[i] = pair
	}

	ok, err := util.ReadBool(rd)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		lastMessage = new(crypto.SignaturePair)
		if err = lastMessage.Decode(c, rd); err != nil {
			return nil, nil, err
		}
	}
	return lastSignatures, lastMessage, nil
}

var _ proto.Packet = (*KeyedPlayerChat)(nil)

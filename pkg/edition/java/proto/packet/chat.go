package packet

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	MaxServerBoundMessageLength = 256
)

type LegacyChat struct {
	Message string
	Type    MessageType
	Sender  uuid.UUID // 1.16+, and can be empty UUID, all zeros
}

func (ch *LegacyChat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteString(wr, ch.Message)
	if err != nil {
		return err
	}
	if c.Direction == proto.ClientBound && c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		err = util.WriteByte(wr, byte(ch.Type))
		if err != nil {
			return err
		}
		if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
			err = util.WriteUUID(wr, ch.Sender)
		}
	}
	return err
}

func (ch *LegacyChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	ch.Message, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	if c.Direction == proto.ClientBound && c.Protocol.GreaterEqual(version.Minecraft_1_8) {
		var pos byte
		pos, err = util.ReadByte(rd)
		if err != nil {
			return err
		}
		ch.Type = MessageType(pos)
		if c.Protocol.GreaterEqual(version.Minecraft_1_16) {
			ch.Sender, err = util.ReadUUID(rd)
			if err != nil {
				return err
			}
		}
	}
	return
}

// MessageType is the position a chat message is going to be sent.
type MessageType byte

const (
	// ChatMessageType lets the chat message appear in the client's HUD.
	// These messages can be filtered out by the client's settings.
	ChatMessageType MessageType = iota
	// SystemMessageType lets the chat message appear in the client's HUD and can't be dismissed.
	SystemMessageType
	// GameInfoMessageType lets the chat message appear above the player's main HUD.
	// This text format doesn't support many component features, such as hover events.
	GameInfoMessageType
)

var _ proto.Packet = (*LegacyChat)(nil)

type PlayerChat struct {
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

func (p *PlayerChat) Encode(c *proto.PacketContext, wr io.Writer) error {
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

func (p *PlayerChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
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

func (p *PlayerChat) SignedContainer(signer crypto.IdentifiedKey, sender uuid.UUID, mustSign bool) (*crypto.SignedChatMessage, error) {
	if p.Unsigned {
		if mustSign {
			return nil, errInvalidSignature
		}
		return nil, nil
	}
	return &crypto.SignedChatMessage{
		Message:            p.Message,
		Signer:             signer.SignedPublicKey(),
		Signature:          p.Signature,
		Expiry:             p.Expiry,
		Salt:               p.Salt,
		Sender:             sender,
		SignedPreview:      p.SignedPreview,
		PreviousSignatures: p.PreviousMessages,
		LastSignature:      p.LastMessage,
	}, nil
}

var _ proto.Packet = (*PlayerChat)(nil)

type PlayerChatPreview struct {
	ID    int
	Query string
}

func (p *PlayerChatPreview) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt(wr, p.ID)
	if err != nil {
		return err
	}
	return util.WriteString(wr, p.Query)
}

func (p *PlayerChatPreview) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.ID, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	p.Query, err = util.ReadStringMax(rd, MaxServerBoundMessageLength)
	return
}

var _ proto.Packet = (*PlayerChatPreview)(nil)

type SystemChat struct {
	Component component.Component
	Type      MessageType
}

func (p *SystemChat) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteComponent(wr, c.Protocol, p.Component)
	if err != nil {
		return err
	}
	if c.Protocol.GreaterEqual(version.Minecraft_1_19_1) {
		switch p.Type {
		case SystemMessageType:
			err = util.WriteBool(wr, true)
		case GameInfoMessageType:
			err = util.WriteBool(wr, false)
		default:
			return fmt.Errorf("invalid chat type: %d", p.Type)
		}
		return err
	}
	return util.WriteVarInt(wr, int(p.Type))
}

func (p *SystemChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.Component, err = util.ReadComponent(rd, c.Protocol)
	if err != nil {
		return err
	}
	// System chat is never decoded so this doesn't matter for now
	typ, err := util.ReadVarInt(rd)
	p.Type = MessageType(typ)
	return
}

var _ proto.Packet = (*SystemChat)(nil)

type ServerChatPreview struct {
	ID      int
	Preview component.Component
}

func (p *ServerChatPreview) Encode(c *proto.PacketContext, wr io.Writer) error {
	err := util.WriteInt(wr, p.ID)
	if err != nil {
		return err
	}
	err = util.WriteBool(wr, p.Preview != nil)
	if err != nil {
		return err
	}
	if p.Preview != nil {
		err = util.WriteComponent(wr, c.Protocol, p.Preview)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *ServerChatPreview) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	p.ID, err = util.ReadInt(rd)
	if err != nil {
		return err
	}
	ok, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		p.Preview, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	}
	return
}

var _ proto.Packet = (*ServerChatPreview)(nil)

type ServerPlayerChat struct {
	Component         component.Component
	UnsignedComponent component.Component // nil-able
	Type              int

	Sender     uuid.UUID
	SenderName component.Component
	TeamName   component.Component // nil-able

	Expiry time.Time
}

func (s *ServerPlayerChat) Encode(c *proto.PacketContext, wr io.Writer) error {
	// TODO TBD
	return errors.New("not yet implemented")
}

func (s *ServerPlayerChat) Decode(c *proto.PacketContext, rd io.Reader) (err error) {
	s.Component, err = util.ReadComponent(rd, c.Protocol)
	if err != nil {
		return err
	}
	ok, err := util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		s.Component, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
		s.UnsignedComponent = s.Component
	}
	s.Type, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	s.SenderName, err = util.ReadComponent(rd, c.Protocol)
	if err != nil {
		return err
	}
	ok, err = util.ReadBool(rd)
	if err != nil {
		return err
	}
	if ok {
		s.TeamName, err = util.ReadComponent(rd, c.Protocol)
		if err != nil {
			return err
		}
	}

	s.Expiry, err = util.ReadUnixMilli(rd)
	if err != nil {
		return err
	}

	salt, err := util.ReadInt64(rd)
	if err != nil {
		return err
	}
	_ = salt
	signature, err := util.ReadBytes(rd)
	if err != nil {
		return err
	}
	_ = signature

	return nil
}

var _ proto.Packet = (*ServerPlayerChat)(nil)

type ChatBuilder struct {
	protocol          proto.Protocol
	component         component.Component
	message           string
	signedChatMessage *crypto.SignedChatMessage
	signedCommand     *crypto.SignedChatCommand
	typ               MessageType
	sender            *uuid.UUID
	timestamp         time.Time
}

func NewChatBuilder(version proto.Protocol) *ChatBuilder {
	return &ChatBuilder{
		protocol:  version,
		timestamp: time.Now(),
	}
}

func (b *ChatBuilder) Message(msg string) *ChatBuilder {
	b.message = msg
	return b
}
func (b *ChatBuilder) Component(c component.Component) *ChatBuilder {
	b.component = c
	return b
}
func (b *ChatBuilder) SignedChatMessage(msg *crypto.SignedChatMessage) *ChatBuilder {
	b.message = msg.Message
	b.signedChatMessage = msg
	return b
}
func (b *ChatBuilder) SignedCommandMessage(cmd *crypto.SignedChatCommand) *ChatBuilder {
	b.message = cmd.Command // root literal
	b.signedCommand = cmd
	return b
}
func (b *ChatBuilder) Type(t MessageType) *ChatBuilder {
	b.typ = t
	return b
}
func (b *ChatBuilder) Time(timestamp time.Time) *ChatBuilder {
	b.timestamp = timestamp
	return b
}
func (b *ChatBuilder) AsPlayer(sender uuid.UUID) *ChatBuilder {
	b.sender = &sender
	return b
}
func (b *ChatBuilder) AsServer() *ChatBuilder {
	b.sender = nil
	return b
}

// ToClient creates a packet which can be sent to the client;
// using the provided information in the builder.
func (b *ChatBuilder) ToClient() proto.Packet {
	msg := b.component
	if msg == nil {
		msg = &component.Text{Content: b.message}
	}

	if b.protocol.GreaterEqual(version.Minecraft_1_19) {
		// hard override chat > system for now
		t := b.typ
		if t == ChatMessageType {
			t = SystemMessageType
		}
		return &SystemChat{
			Component: msg,
			Type:      t,
		}
	}

	id := uuid.Nil
	if b.sender != nil {
		id = *b.sender
	}
	buf := new(strings.Builder)
	_ = util.JsonCodec(b.protocol).Marshal(buf, msg)
	return &LegacyChat{
		Message: buf.String(),
		Type:    b.typ,
		Sender:  id,
	}
}

// ToServer creates a packet which can be sent to the server;
// using the provided information in the builder.
func (b *ChatBuilder) ToServer() proto.Packet {
	if b.protocol.GreaterEqual(version.Minecraft_1_19) {
		if b.signedChatMessage != nil {
			return toPlayerChat(b.signedChatMessage)
		}
		if b.signedCommand != nil {
			return toPlayerCommand(b.signedCommand)
		}
		// Well crap
		if strings.HasPrefix(b.message, "/") {
			return NewPlayerCommand(
				strings.TrimPrefix(b.message, "/"),
				nil, time.Now())
		}
		// This will produce an error on the server, but needs to be here.
		return &PlayerChat{
			Message:  b.message,
			Unsigned: true,
		}
	}
	return &LegacyChat{Message: b.message}
}

func toPlayerChat(m *crypto.SignedChatMessage) *PlayerChat {
	var lastMsg *crypto.SignaturePair
	if len(m.PreviousSignatures) != 0 {
		lastMsg = m.PreviousSignatures[0]
	}
	return &PlayerChat{
		Message:          m.Message,
		SignedPreview:    m.SignedPreview,
		Unsigned:         false,
		Expiry:           m.Expiry,
		Signature:        m.Signature,
		Salt:             m.Salt,
		LastMessage:      lastMsg,
		PreviousMessages: m.PreviousSignatures,
	}
}

func toPlayerCommand(m *crypto.SignedChatCommand) *PlayerCommand {
	salt, _ := util.ReadInt64(bytes.NewReader(m.Salt))
	return &PlayerCommand{
		Unsigned:      false,
		Command:       m.Command,
		Timestamp:     time.Time{},
		Salt:          salt,
		SignedPreview: m.SignedPreview,
		Arguments:     nil,
	}
}

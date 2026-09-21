package packet

import (
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"io"
	"math"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

// HandshakeIntent represents the client intent in the Handshake state.
type HandshakeIntent int

const (
	StatusHandshakeIntent   = HandshakeIntent(states.StatusState)
	LoginHandshakeIntent    = HandshakeIntent(states.LoginState)
	TransferHandshakeIntent = HandshakeIntent(3)
)

// https://wiki.vg/Protocol#Handshaking
type Handshake struct {
	ProtocolVersion int
	ServerAddress   string
	// Port is the client-declared server port. It is an UNSIGNED 16-bit field on
	// the wire (0-65535) and is always kept in that range, so ports >= 32768 do
	// not come back negative (65535 used to decode as -1) and the value is safe
	// to format into the virtual host used for routing.
	Port       int
	NextStatus int
}

func (h *Handshake) Intent() HandshakeIntent {
	switch h.NextStatus {
	case 1:
		return StatusHandshakeIntent
	case 2:
		return LoginHandshakeIntent
	case 3:
		return TransferHandshakeIntent
	default:
		panic(fmt.Errorf("unsupported next status %v -> handshake intent", h.NextStatus))
	}
}

func (h *Handshake) Encode(_ *proto.PacketContext, wr io.Writer) error {
	// The port is written as an unsigned 16-bit value, so anything outside that
	// range is rejected explicitly instead of being silently narrowed into a
	// different port on the wire.
	if h.Port < 0 || h.Port > math.MaxUint16 {
		return fmt.Errorf("handshake port %d out of range (0-%d)", h.Port, math.MaxUint16)
	}
	err := util.WriteVarInt(wr, h.ProtocolVersion)
	if err != nil {
		return err
	}
	err = util.WriteString(wr, h.ServerAddress)
	if err != nil {
		return err
	}
	err = util.WriteUint16(wr, uint16(h.Port))
	if err != nil {
		return err
	}
	return util.WriteVarInt(wr, h.NextStatus)
}

func (h *Handshake) Decode(_ *proto.PacketContext, rd io.Reader) (err error) {
	h.ProtocolVersion, err = util.ReadVarInt(rd)
	if err != nil {
		return err
	}
	h.ServerAddress, err = util.ReadString(rd)
	if err != nil {
		return err
	}
	port, err := util.ReadUint16(rd)
	if err != nil {
		return err
	}
	h.Port = int(port)
	h.NextStatus, err = util.ReadVarInt(rd)
	return err
}

var _ proto.Packet = (*Handshake)(nil)

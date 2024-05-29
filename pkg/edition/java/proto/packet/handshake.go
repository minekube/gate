package packet

import (
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"io"

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
	Port            int
	NextStatus      int
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
	err := util.WriteVarInt(wr, h.ProtocolVersion)
	if err != nil {
		return err
	}
	err = util.WriteString(wr, h.ServerAddress)
	if err != nil {
		return err
	}
	err = util.WriteInt16(wr, int16(h.Port))
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
	port, err := util.ReadInt16(rd)
	if err != nil {
		return err
	}
	h.Port = int(port)
	h.NextStatus, err = util.ReadVarInt(rd)
	return err
}

var _ proto.Packet = (*Handshake)(nil)

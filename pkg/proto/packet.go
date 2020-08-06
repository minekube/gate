package proto

import (
	"fmt"
	"io"
	"reflect"
)

// Packet should be implemented by any Minecraft protocol packet.
// It is the layer of the protocol packet's data.
type Packet interface {
	Encode(c *PacketContext, wr io.Writer) error       // Encodes the packet into the writer
	Decode(c *PacketContext, rd io.Reader) (err error) // Decodes a packet by reading from the reader
}

// TypeOf is a helper func to make sure the
// reflect.Type of p implements Packet
// and returns a non-pointer type.
func TypeOf(p Packet) PacketType {
	t := reflect.TypeOf(p)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

type PacketContext struct {
	Direction Direction // The direction the packet was bound to.
	Protocol  Protocol  // The protocol version of the packet.

	KnownPacket bool     // If false Packet field is nil.
	PacketId    PacketId // Is always set.
	Packet      Packet   // The decoded packet.

	// The unencrypted and uncompressed form of packet id + data.
	// It contains the actual received payload (may be longer than what the Packet's Decode read).
	// This can be used to skip encoding Packet.
	Payload []byte // Empty when encoding.
}

// Direction is the direction a packet is meant to go to/come from.
type Direction uint8

const (
	ClientBound Direction = iota // Packets sent to the client.
	ServerBound                  // Packets send to this proxy.
)

func (d Direction) String() string {
	switch d {
	case ServerBound:
		return "ServerBound"
	case ClientBound:
		return "ClientBound"
	}
	return "UnknownBound"
}

// State is a client state.
type State int

// States the client connection can be in.
const (
	HandshakeState State = iota
	StatusState
	LoginState
	PlayState
)

func (s State) String() string {
	switch s {
	case StatusState:
		return "Status"
	case HandshakeState:
		return "Handshake"
	case LoginState:
		return "Login"
	case PlayState:
		return "Play"
	}
	return "UnknownState"
}

// PacketId identifies a packet.
type PacketId int

func (p PacketId) String() string {
	return fmt.Sprintf("%x", int(p))
}

// PacketType helps to instantiate a new packet of it's reflect type.
type PacketType reflect.Type

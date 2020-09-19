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

// PacketContext carries context information for a received packet.
type PacketContext struct {
	Direction Direction // The direction the packet is bound to.
	Protocol  Protocol  // The protocol version of the packet.
	PacketID  PacketID  // The ID of the packet, is always set.

	// Whether the PacketID is known in the connections current state.ProtocolRegistry.
	// If true Packet field is non-nil.
	KnownPacket bool

	// Is the decoded type that is found by PacketID in the connections
	// current state.ProtocolRegistry. Otherwise is nil and an unknown
	// PacketID and is probably to be forwarded.
	Packet Packet

	// The unencrypted and uncompressed form of packet id + data.
	// It contains the actual received payload (may be longer than what the Packet's Decode read).
	// This can be used to skip encoding Packet.
	Payload []byte // Empty when encoding.
}

func (c *PacketContext) String() string {
	return fmt.Sprintf("PacketContext:direction=%s,Protocol=%s,"+
		"KnownPacket=%t,PacketID=%s,PacketType=%s,Payloadlen=%d",
		c.Direction, c.Protocol, c.KnownPacket, c.PacketID,
		reflect.TypeOf(c.Packet), len(c.Payload))
}

// Direction is the direction a packet is bound to.
//  - Receiving a packet from a client is ServerBound.
//  - Receiving a packet from a server is ClientBound.
//  - Sending a packet to a client is ClientBound.
//  - Sending a packet to a server is ServerBound.
type Direction uint8

// Available packet bound directions.
const (
	ClientBound Direction = iota // A packet is send to a client.
	ServerBound                  // A packet is send to a server.
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

// PacketID identifies a packet in a protocol version.
type PacketID int

func (id PacketID) String() string {
	return fmt.Sprintf("%x", int(id))
}

// PacketType is the non-pointer reflect.Type of a packet.
// Use TypeOf helper function to for convenience.
type PacketType reflect.Type

// TypeOf returns a non-pointer type of p.
func TypeOf(p Packet) PacketType {
	t := reflect.TypeOf(p)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

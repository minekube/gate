package proto

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

// Indicates a packet was known and successfully decoded by it's registered decoder,
// but the decoder has not read all of the packet's data.
//
// This may happen in cases where
//  - the decoder has a bug
//  - the decoder does not handle the case for the new protocol version of the packet changed by Mojang/Minecraft
//  - someone (server/client) has sent valid bytes in the beginning of the packet's data that the packet's
//    decoder could successfully decode, but then the data contains even more bytes (the left bytes)
var ErrDecoderLeftBytes = errors.New("decoder did not read all packet data")

// Packet should be implemented by all Minecraft packet types.
// It is the data layer of a packet and shall support multiple protocols
// up- and/or downwards by testing the Protocol contained in the passed PacketContext.
// The passed PacketContext is read-only and must not be modified.
type Packet interface {
	Encode(c *PacketContext, wr io.Writer) error       // Encode encodes the packet data into the writer.
	Decode(c *PacketContext, rd io.Reader) (err error) // Decodes expected data from a reader into the packet.
}

// PacketContext carries context information for a
// received packet or packet that is about to be send.
type PacketContext struct {
	Direction Direction // The direction the packet is bound to.
	Protocol  Protocol  // The protocol version of the packet.
	PacketID  PacketID  // The ID of the packet, is always set.

	// Whether the PacketID is known in the connection's current state.ProtocolRegistry.
	// If false field Packet is nil, which in most cases indicates a forwarded packet that
	// is just going to be proxy-ed through to client <--> backend connection.
	KnownPacket bool

	// Is the decoded type that is found by PacketID in the connections
	// current state.ProtocolRegistry. Otherwise nil, the PacketID is unknown
	// and KnownPacket is false.
	Packet Packet

	// The unencrypted and uncompressed form of packet id + data.
	// It contains the actual received payload (may be longer than what the Packet's Decode read).
	// This can be used to skip encoding Packet.
	Payload []byte // Empty when encoding.
}

// PacketID identifies a packet in a protocol version.
// PacketIDs vary by Protocol version and different
// packet types exist in each Minecraft edition.
type PacketID int

// String implements fmt.Stringer.
func (id PacketID) String() string {
	return fmt.Sprintf("%x", int(id))
}

// String implements fmt.Stringer.
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
	ClientBound Direction = iota // A packet is bound to a client.
	ServerBound                  // A packet is bound to a server.
)

// String implements fmt.Stringer.
func (d Direction) String() string {
	switch d {
	case ServerBound:
		return "ServerBound"
	case ClientBound:
		return "ClientBound"
	}
	return "UnknownBound"
}

// Version is a Minecraft edition agnostic named protocol version.
type Version struct {
	Protocol
	Name string
}

// Protocol is a Minecraft edition agnostic protocol version id specified by Mojang.
type Protocol int

// String implements fmt.Stringer.
func (p Protocol) String() string {
	return strconv.Itoa(int(p))
}

// String implements fmt.Stringer.
func (v Version) String() string {
	return v.Name
}

// GreaterEqual is true when this Protocol is
// greater or equal then another Version's Protocol.
func (p Protocol) GreaterEqual(then *Version) bool {
	return p >= then.Protocol
}

// LowerEqual is true when this Protocol is
// lower or equal then another Version's Protocol.
func (p Protocol) LowerEqual(then *Version) bool {
	return p <= then.Protocol
}

// Lower is true when this Protocol is
// lower then another Version's Protocol.
func (p Protocol) Lower(then *Version) bool {
	return p < then.Protocol
}

// Greater is true when this Protocol is
// greater then another Version's Protocol.
func (p Protocol) Greater(then *Version) bool {
	return p > then.Protocol
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

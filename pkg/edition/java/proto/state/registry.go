package state

import (
	"fmt"
	"reflect"

	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Registry stores server/client bound packets for a specific State.
type Registry struct {
	State
	ServerBound *PacketRegistry
	ClientBound *PacketRegistry
}

// NewRegistry returns a new state registry.
func NewRegistry(state State) *Registry {
	return &Registry{
		State:       state,
		ServerBound: NewPacketRegistry(proto.ServerBound),
		ClientBound: NewPacketRegistry(proto.ClientBound),
	}
}

// PacketRegistry stores packets protocol versions sent to server or client.
type PacketRegistry struct {
	Direction proto.Direction                      // The direction the registered packets are send to.
	Protocols map[proto.Protocol]*ProtocolRegistry // The protocol versions.
	// Whether to fallback to the minimum protocol version
	// in case a protocol could not be found.
	Fallback bool
}

func NewPacketRegistry(direction proto.Direction) *PacketRegistry {
	r := &PacketRegistry{
		Direction: direction,
		Protocols: map[proto.Protocol]*ProtocolRegistry{},
		Fallback:  true, // fallback by default
	}
	for _, ver := range version.Versions {
		if !version.Protocol(ver.Protocol).Legacy() && !version.Protocol(ver.Protocol).Unknown() {
			r.Protocols[ver.Protocol] = &ProtocolRegistry{
				Protocol:    ver.Protocol,
				PacketIDs:   map[proto.PacketID]proto.PacketType{},
				PacketTypes: map[proto.PacketType]proto.PacketID{},
			}
		}
	}
	return r
}

// ProtocolRegistry gets the ProtocolRegistry for a protocol.
func (p *PacketRegistry) ProtocolRegistry(protocol proto.Protocol) *ProtocolRegistry {
	r := p.Protocols[protocol]
	if r == nil && p.Fallback {
		return p.ProtocolRegistry(version.MinimumVersion.Protocol)
	}
	return r // nil if not found
}

// ProtocolRegistry stores packets of a protocol version.
type ProtocolRegistry struct {
	Protocol    proto.Protocol                      // The protocol version of the registered packets.
	PacketIDs   map[proto.PacketID]proto.PacketType // Gets packet type by packet id.
	PacketTypes map[proto.PacketType]proto.PacketID // Gets packet id by packet type.
}

// PacketID gets the packet id by the registered packet type.
func (r *ProtocolRegistry) PacketID(of proto.Packet) (id proto.PacketID, found bool) {
	id, found = r.PacketTypes[proto.TypeOf(of)]
	return
}

// CreatePacket returns a new zero valued instance of the type
// of the mapped packet id or nil if not found.
func (r *ProtocolRegistry) CreatePacket(id proto.PacketID) proto.Packet {
	packetType, ok := r.PacketIDs[id]
	if !ok {
		return nil
	}
	p, ok := reflect.New(packetType).Interface().(proto.Packet)
	if !ok {
		// Shall never happen...
		// Tried to create packet that does not implement Packet interface
		return nil
	}
	return p
}

func (p *PacketRegistry) Register(packetOf proto.Packet, mappings ...*PacketMapping) {
	packetType := proto.TypeOf(packetOf)

	var (
		next *PacketMapping
		from proto.Protocol
		to   proto.Protocol
	)
	for i, current := range mappings {
		from = current.Protocol
		lastValid := current.LastValidProtocol
		if lastValid != 0 {
			if next != current {
				panic("Cannot add a mapping after last valid mapping")
			}
			if from > lastValid {
				panic("Last mapping version cannot be higher than highest mapping version")
			}
		}
		// set "next"
		if i < len(mappings)-1 {
			next = mappings[i+1]
		} else {
			next = current
		}
		// set "to"
		if current == next {
			if lastValid != 0 {
				to = lastValid
			} else {
				to = version.MaximumVersion.Protocol
			}
		} else {
			to = next.Protocol
		}

		if from >= to && from != version.MaximumVersion.Protocol {
			panic(fmt.Sprintf("Next mapping version (%s) should be lower then current (%s)", to, from))
		}

		versionRange(version.Versions, from, to, func(protocol proto.Protocol) bool {
			if protocol == to && next != current {
				return false
			}
			registry, ok := p.Protocols[protocol]
			if !ok {
				panic(fmt.Sprintf("Unknown protocol version %s", current.Protocol))
			}

			if _, ok = registry.PacketIDs[current.ID]; ok {
				panic(fmt.Sprintf("Can not register packet type %s with id %#x for "+
					"protocol %s because another packet is already registered", packetType, current.ID, registry.Protocol))
			}
			if _, ok = registry.PacketTypes[packetType]; ok {
				panic(fmt.Sprintf("%T is already registered for protocol %s", packetOf, registry.Protocol))
			}
			registry.PacketIDs[current.ID] = packetType
			registry.PacketTypes[packetType] = current.ID
			return true
		})
	}
}

func FromDirection(direction proto.Direction, state *Registry, protocol proto.Protocol) *ProtocolRegistry {
	if direction == proto.ServerBound {
		return state.ServerBound.ProtocolRegistry(protocol)
	}
	return state.ClientBound.ProtocolRegistry(protocol)
}

type PacketMapping struct {
	ID                proto.PacketID
	Protocol          proto.Protocol
	LastValidProtocol proto.Protocol // defaults to version.MaximumVersion.Protocol
}

// m returns new PacketMapping
func m(id proto.PacketID, version *proto.Version) *PacketMapping {
	return ml(id, version, nil)
}

// ml returns new PacketMapping with optional lastValidProtocolVersion
//
//  id         packet Id
//  version    protocol version
//  encodeOnly when true packet decoding will be disabled
//  lastValidProtocolVersion last version this mapping is valid at
func ml(id proto.PacketID, version, lastValidProtocol *proto.Version) *PacketMapping {
	var last proto.Protocol
	if lastValidProtocol != nil {
		last = lastValidProtocol.Protocol
	}
	return &PacketMapping{
		ID:                id,
		Protocol:          version.Protocol,
		LastValidProtocol: last,
	}
}

func versionRange(
	versions []*proto.Version,
	from, to proto.Protocol,
	fn func(p proto.Protocol,
	) bool) {
	var inRange bool
	for _, ver := range versions {
		if ver.Protocol == from {
			inRange = true
		} else if ver.Protocol == to {
			fn(ver.Protocol)
			return
		}
		if inRange {
			if !fn(ver.Protocol) {
				return
			}
		}
	}
}

// String implements fmt.Stringer.
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

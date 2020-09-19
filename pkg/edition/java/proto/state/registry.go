package state

import (
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/runtime/logr"
	"reflect"
)

// Registry stores server/client bound packets.
type Registry struct {
	proto.State
	ServerBound *PacketRegistry
	ClientBound *PacketRegistry
}

func NewRegistry(state proto.State) *Registry {
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
	for _, ver := range proto.Versions {
		if !ver.Legacy() && !ver.Unknown() {
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
		return p.ProtocolRegistry(proto.MinimumVersion.Protocol)
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

var log = logr.Log.WithName("proto-registry")

// CreatePacket returns a new zero valued instance of the type
// of the mapped packet id or nil if not found.
func (r *ProtocolRegistry) CreatePacket(id proto.PacketID) proto.Packet {
	packetType, ok := r.PacketIDs[id]
	if !ok {
		return nil
	}
	p, ok := reflect.New(packetType).Interface().(proto.Packet)
	if !ok {
		// Shall not happen, but let's be extra sure
		log.Info("Tried to create packet that does not implement Packet interface",
			"type", packetType, "id", id)
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
		if i < len(mappings)-1 {
			next = mappings[i+1]
			to = next.Protocol
		} else {
			next = current
			to = proto.MaximumVersion.Protocol
		}

		if from >= to && from != proto.MaximumVersion.Protocol {
			panic(fmt.Sprintf("Next mapping version (%s) should be lower then current (%s)", to, from))
		}

		versionRange(proto.Versions, from, to, func(protocol proto.Protocol) bool {
			if protocol == to && next != current {
				return false
			}
			registry, ok := p.Protocols[protocol]
			if !ok {
				panic(fmt.Sprintf("Unknown protocol version %s", current.Protocol))
			}

			if _, ok = registry.PacketIDs[current.ID]; ok {
				panic(fmt.Sprintf("Can not register packet type %T with id %#x for "+
					"protocol %s because another packet is already registered", packetOf, current.ID, registry.Protocol))
			}
			if _, ok = registry.PacketTypes[proto.TypeOf(packetOf)]; ok {
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
	ID       proto.PacketID
	Protocol proto.Protocol
}

func m(id proto.PacketID, version *proto.Version) *PacketMapping {
	return &PacketMapping{
		ID:       id,
		Protocol: version.Protocol,
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

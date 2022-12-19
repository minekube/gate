package brigadier

import (
	"errors"
	"fmt"
	"io"

	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

var registry = &argPropReg{
	byId:         map[string]ArgumentPropertyCodec{},
	byType:       map[string]ArgumentPropertyCodec{},
	typeToID:     map[string]*ArgumentIdentifier{},
	byIdentifier: map[string]*ArgumentIdentifier{},
}

// argument property registry
type argPropReg struct {
	byId         map[string]ArgumentPropertyCodec
	byType       map[string]ArgumentPropertyCodec
	typeToID     map[string]*ArgumentIdentifier
	byIdentifier map[string]*ArgumentIdentifier
}

func (r *argPropReg) empty(identifier *ArgumentIdentifier, codec ArgumentPropertyCodec) {
	r.byId[identifier.id] = codec
	r.byIdentifier[identifier.id] = identifier
}

func (r *argPropReg) register(
	identifier *ArgumentIdentifier,
	argType brigodier.ArgumentType,
	codec ArgumentPropertyCodec,
) {
	r.byIdentifier[identifier.id] = identifier
	r.byId[identifier.id] = codec
	r.byType[argType.String()] = codec
	r.typeToID[argType.String()] = identifier
}

func Encode(wr io.Writer, argType brigodier.ArgumentType, protocol proto.Protocol) error {
	return registry.Encode(wr, argType, protocol)
}
func (r *argPropReg) Encode(wr io.Writer, argType brigodier.ArgumentType, protocol proto.Protocol) error {
	switch property := argType.(type) {
	case *passthroughProperty:
		err := r.writeIdentifier(wr, property.identifier, protocol)
		if err != nil {
			return err
		}
		return nil
	case *ModArgumentProperty:
		err := r.writeIdentifier(wr, property.Identifier, protocol)
		if err != nil {
			return err
		}
		return util.WriteBytes(wr, property.Data)
	default:
		codec, ok := r.byType[argType.String()]
		id, ok2 := r.typeToID[argType.String()]
		if !ok || !ok2 {
			return fmt.Errorf("don't know how to encode %T", argType)
		}
		err := r.writeIdentifier(wr, id, protocol)
		if err != nil {
			return err
		}
		return codec.Encode(wr, argType, protocol)
	}
}

func Decode(rd io.Reader, protocol proto.Protocol) (brigodier.ArgumentType, error) {
	return registry.Decode(rd, protocol)
}
func (r *argPropReg) Decode(rd io.Reader, protocol proto.Protocol) (brigodier.ArgumentType, error) {
	identifier, err := r.readIdentifier(rd, protocol)
	if err != nil {
		return nil, err
	}
	codec := r.byId[identifier.id]
	if codec == nil {
		return nil, fmt.Errorf("unknown argument type identifier %q", identifier)
	}
	result, err := codec.Decode(rd, protocol)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &passthroughProperty{
			identifier: identifier,
		}, nil
	}
	return result, nil
}

type passthroughProperty struct{ identifier *ArgumentIdentifier }

var _ brigodier.ArgumentType = (*passthroughProperty)(nil)

func (p *passthroughProperty) Parse(rd *brigodier.StringReader) (any, error) {
	return nil, errors.New("unsupported operation")
}
func (p *passthroughProperty) String() string { return p.identifier.id }

func (r *argPropReg) writeIdentifier(wr io.Writer, identifier *ArgumentIdentifier, protocol proto.Protocol) error {
	if protocol.GreaterEqual(version.Minecraft_1_19) {
		id, ok := identifier.idByProtocol[protocol]
		if !ok {
			return fmt.Errorf("don't know how to encode type %s", identifier)
		}
		return util.WriteVarInt(wr, id)
	}
	return util.WriteString(wr, identifier.id)
}

var errIdentifierNotFound = errors.New("identifier not found")

func (r *argPropReg) readIdentifier(rd io.Reader, protocol proto.Protocol) (*ArgumentIdentifier, error) {
	if protocol.GreaterEqual(version.Minecraft_1_19) {
		id, err := util.ReadVarInt(rd)
		if err != nil {
			return nil, err
		}
		for _, i := range r.byIdentifier {
			v, ok := i.idByProtocol[protocol]
			if ok && v == id {
				return i, nil
			}
		}
	} else {
		identifier, err := util.ReadString(rd)
		if err != nil {
			return nil, err
		}
		i, ok := r.byIdentifier[identifier]
		if ok {
			return i, nil
		}
	}
	return nil, errIdentifierNotFound
}

func init() {
	register := registry.register
	emptyWithCodec := registry.empty
	empty := func(id *ArgumentIdentifier) { emptyWithCodec(id, EmptyArgumentPropertyCodec) }
	id := func(id string, versions ...versionSet) *ArgumentIdentifier {
		i, err := newArgIdentifier(id, versions...)
		if err != nil {
			panic(fmt.Errorf("could not create argument identifier %s: %w", id, err))
		}
		return i
	}
	mapSet := func(version *proto.Version, id int) versionSet {
		return versionSet{
			version: version.Protocol,
			id:      id,
		}
	}

	// Base Brigadier argument types
	register(id("brigadier:bool", mapSet(version.Minecraft_1_19, 0)), brigodier.Bool, BoolArgumentPropertyCodec)
	register(id("brigadier:float", mapSet(version.Minecraft_1_19, 1)), brigodier.Float32, Float32ArgumentPropertyCodec)
	register(id("brigadier:double", mapSet(version.Minecraft_1_19, 2)), brigodier.Float64, Float64ArgumentPropertyCodec)
	register(id("brigadier:integer", mapSet(version.Minecraft_1_19, 3)), brigodier.Int, Int32ArgumentPropertyCodec)
	register(id("brigadier:long", mapSet(version.Minecraft_1_19, 4)), brigodier.Int64, Int64ArgumentPropertyCodec)
	register(id("brigadier:string", mapSet(version.Minecraft_1_19, 5)), brigodier.String, StringArgumentPropertyCodec)

	// Minecraft argument types
	register(id("minecraft:entity", mapSet(version.Minecraft_1_19, 6)), ByteArgumentType(0), ByteArgumentPropertyCodec)
	empty(id("minecraft:game_profile", mapSet(version.Minecraft_1_19, 7)))
	empty(id("minecraft:block_pos", mapSet(version.Minecraft_1_19, 8)))
	empty(id("minecraft:column_pos", mapSet(version.Minecraft_1_19, 9)))
	empty(id("minecraft:vec3", mapSet(version.Minecraft_1_19, 10)))
	empty(id("minecraft:vec2", mapSet(version.Minecraft_1_19, 11)))
	empty(id("minecraft:block_state", mapSet(version.Minecraft_1_19, 12)))
	empty(id("minecraft:block_predicate", mapSet(version.Minecraft_1_19, 13)))
	empty(id("minecraft:item_stack", mapSet(version.Minecraft_1_19, 14)))
	empty(id("minecraft:item_predicate", mapSet(version.Minecraft_1_19, 15)))
	empty(id("minecraft:color", mapSet(version.Minecraft_1_19, 16)))
	empty(id("minecraft:component", mapSet(version.Minecraft_1_19, 17)))
	empty(id("minecraft:message", mapSet(version.Minecraft_1_19, 18)))
	empty(id("minecraft:nbt_compound_tag", mapSet(version.Minecraft_1_19, 19))) // added in 1.14
	empty(id("minecraft:nbt_tag", mapSet(version.Minecraft_1_19, 20)))          // added in 1.14
	empty(id("minecraft:nbt_path", mapSet(version.Minecraft_1_19, 21)))
	empty(id("minecraft:objective", mapSet(version.Minecraft_1_19, 22)))
	empty(id("minecraft:objective_criteria", mapSet(version.Minecraft_1_19, 23)))
	empty(id("minecraft:operation", mapSet(version.Minecraft_1_19, 24)))
	empty(id("minecraft:particle", mapSet(version.Minecraft_1_19, 25)))
	empty(id("minecraft:angle", mapSet(version.Minecraft_1_19, 26))) // added in 1.16.2
	empty(id("minecraft:rotation", mapSet(version.Minecraft_1_19, 27)))
	empty(id("minecraft:scoreboard_slot", mapSet(version.Minecraft_1_19, 28)))
	register(id("minecraft:score_holder", mapSet(version.Minecraft_1_19, 29)), ByteArgumentType(0), ByteArgumentPropertyCodec)
	empty(id("minecraft:swizzle", mapSet(version.Minecraft_1_19, 30)))
	empty(id("minecraft:team", mapSet(version.Minecraft_1_19, 31)))
	empty(id("minecraft:item_slot", mapSet(version.Minecraft_1_19, 32)))
	empty(id("minecraft:resource_location", mapSet(version.Minecraft_1_19, 33)))
	empty(id("minecraft:mob_effect", mapSet(version.Minecraft_1_19_3, -1), mapSet(version.Minecraft_1_19, 34)))
	empty(id("minecraft:function", mapSet(version.Minecraft_1_19_3, 34), mapSet(version.Minecraft_1_19, 35)))
	empty(id("minecraft:entity_anchor", mapSet(version.Minecraft_1_19_3, 35), mapSet(version.Minecraft_1_19, 36)))
	empty(id("minecraft:int_range", mapSet(version.Minecraft_1_19_3, 36), mapSet(version.Minecraft_1_19, 37)))
	empty(id("minecraft:float_range", mapSet(version.Minecraft_1_19_3, 37), mapSet(version.Minecraft_1_19, 38)))
	empty(id("minecraft:item_enchantment", mapSet(version.Minecraft_1_19_3, -1), mapSet(version.Minecraft_1_19, 39)))
	empty(id("minecraft:entity_summon", mapSet(version.Minecraft_1_19_3, -1), mapSet(version.Minecraft_1_19, 40)))
	empty(id("minecraft:dimension", mapSet(version.Minecraft_1_19_3, 38), mapSet(version.Minecraft_1_19, 41)))
	empty(id("minecraft:gamemode", mapSet(version.Minecraft_1_19_3, 39)))                                 // 1.19.3
	empty(id("minecraft:time", mapSet(version.Minecraft_1_19_3, 40), mapSet(version.Minecraft_1_19, 42))) // added in 1.14

	register(id("minecraft:resource_or_tag", mapSet(version.Minecraft_1_19_3, 41), mapSet(version.Minecraft_1_19, 43)),
		RegistryKeyArgument, RegistryKeyArgumentPropertyCodec)
	register(id("minecraft:resource_or_tag_key", mapSet(version.Minecraft_1_19_3, 42)),
		ResourceOrTagKeyArgument, ResourceOrTagKeyArgumentPropertyCodec)
	register(id("minecraft:resource", mapSet(version.Minecraft_1_19_3, 43), mapSet(version.Minecraft_1_19, 44)),
		RegistryKeyArgument, RegistryKeyArgumentPropertyCodec)
	register(id("minecraft:resource_key", mapSet(version.Minecraft_1_19_3, 44)),
		ResourceKeyArgument, ResourceKeyArgumentPropertyCodec)

	empty(id("minecraft:template_mirror", mapSet(version.Minecraft_1_19, 45)))   // 1.19
	empty(id("minecraft:template_rotation", mapSet(version.Minecraft_1_19, 46))) // 1.19

	empty(id("minecraft:uuid", mapSet(version.Minecraft_1_19, 47))) // added in 1.16

	// Crossstitch support
	register(id("crossstitch:mod_argument", mapSet(version.Minecraft_1_19, -256)), &ModArgumentProperty{}, ModArgumentPropertyCodec)

	empty(id("minecraft:nbt")) // No longer in 1.19+
}

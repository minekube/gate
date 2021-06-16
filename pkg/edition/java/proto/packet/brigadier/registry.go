package brigadier

import (
	"encoding/json"
	"errors"
	"fmt"
	"go.minekube.com/brigodier"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"io"
)

type ArgumentPropertyCodec interface {
	Encode(wr io.Writer, v interface{}) error
	Decode(rd io.Reader) (interface{}, error)
}

var registry = &argPropReg{
	byId:     map[string]ArgumentPropertyCodec{},
	byType:   map[string]ArgumentPropertyCodec{},
	typeToID: map[string]string{},
}

// argument property registry
type argPropReg struct {
	byId     map[string]ArgumentPropertyCodec
	byType   map[string]ArgumentPropertyCodec
	typeToID map[string]string
}

func (r *argPropReg) empty(identifier string, codec ArgumentPropertyCodec) {
	r.byId[identifier] = codec
}

func (r *argPropReg) register(
	identifier string,
	argType brigodier.ArgumentType,
	codec ArgumentPropertyCodec,
) {
	r.byId[identifier] = codec
	r.byType[argType.String()] = codec
	r.typeToID[argType.String()] = identifier
}

func Encode(wr io.Writer, argType brigodier.ArgumentType) error { return registry.Encode(wr, argType) }
func (r *argPropReg) Encode(wr io.Writer, argType brigodier.ArgumentType) error {
	if property, ok := argType.(*PassthroughProperty); ok {
		err := util.WriteString(wr, property.Identifier)
		if err != nil {
			return err
		}
		if property.Result != nil {
			err = property.Codec.Encode(wr, property.Result)
		}
		return err
	}

	codec := r.byType[argType.String()]
	id := r.typeToID[argType.String()]
	if codec == nil || id == "" {
		return fmt.Errorf("don't know how to encode %T (id=%q, codec=%T)", argType, id, codec)
	}
	err := util.WriteString(wr, id)
	if err != nil {
		return err
	}
	return codec.Encode(wr, argType)
}

func Decode(rd io.Reader) (brigodier.ArgumentType, error) { return registry.Decode(rd) }
func (r *argPropReg) Decode(rd io.Reader) (brigodier.ArgumentType, error) {
	identifier, err := util.ReadString(rd)
	if err != nil {
		return nil, err
	}
	codec := r.byId[identifier]
	if codec == nil {
		return nil, fmt.Errorf("unknown argument type identifier %q", identifier)
	}
	result, err := codec.Decode(rd)
	if err != nil {
		return nil, err
	}
	if a, ok := result.(brigodier.ArgumentType); ok {
		return a, nil
	}
	return &PassthroughProperty{
		Identifier: identifier,
		Codec:      codec,
		Result:     result,
	}, nil
}

type PassthroughProperty struct {
	Identifier string
	Codec      ArgumentPropertyCodec
	Result     interface{}
}

var _ brigodier.ArgumentType = (*PassthroughProperty)(nil)

// Parse is unsupported.
func (p *PassthroughProperty) Parse(*brigodier.StringReader) (interface{}, error) {
	return nil, errors.New("calling PassthroughProperty.Parse is an unsupported operation")
}
func (p *PassthroughProperty) String() string {
	j, _ := json.Marshal(struct {
		Identifier string
		Result     interface{}
	}{Identifier: p.Identifier, Result: p.Result})
	return "*PassthroughProperty" + string(j)
}

func init() {
	register := registry.register
	emptyWithCodec := registry.empty
	empty := func(id string) { emptyWithCodec(id, EmptyArgumentPropertyCodec) }

	// Base Brigadier argument types
	register("brigadier:string", brigodier.String, StringArgumentPropertyCodec)
	register("brigadier:integer", brigodier.Int, Int32ArgumentPropertyCodec)
	register("brigadier:long", brigodier.Int64, Int64ArgumentPropertyCodec)
	register("brigadier:float", brigodier.Float32, Float32ArgumentPropertyCodec)
	register("brigadier:double", brigodier.Float64, Float64ArgumentPropertyCodec)
	register("brigadier:bool", brigodier.Bool, BoolArgumentPropertyCodec)

	// Minecraft argument types with extra properties
	emptyWithCodec("minecraft:entity", ByteArgumentPropertyCodec)
	emptyWithCodec("minecraft:score_holder", ByteArgumentPropertyCodec)

	// Minecraft argument types
	empty("minecraft:game_profile")
	empty("minecraft:block_pos")
	empty("minecraft:column_pos")
	empty("minecraft:vec3")
	empty("minecraft:vec2")
	empty("minecraft:block_state")
	empty("minecraft:block_predicate")
	empty("minecraft:item_stack")
	empty("minecraft:item_predicate")
	empty("minecraft:color")
	empty("minecraft:component")
	empty("minecraft:message")
	empty("minecraft:nbt")
	empty("minecraft:nbt_compound_tag") // added in 1.14
	empty("minecraft:nbt_tag")          // added in 1.14
	empty("minecraft:nbt_path")
	empty("minecraft:objective")
	empty("minecraft:objective_criteria")
	empty("minecraft:operation")
	empty("minecraft:particle")
	empty("minecraft:rotation")
	empty("minecraft:scoreboard_slot")
	empty("minecraft:swizzle")
	empty("minecraft:team")
	empty("minecraft:item_slot")
	empty("minecraft:resource_location")
	empty("minecraft:mob_effect")
	empty("minecraft:function")
	empty("minecraft:entity_anchor")
	empty("minecraft:item_enchantment")
	empty("minecraft:entity_summon")
	empty("minecraft:dimension")
	empty("minecraft:int_range")
	empty("minecraft:float_range")
	empty("minecraft:time")  // added in 1.14
	empty("minecraft:uuid")  // added in 1.16
	empty("minecraft:angle") // added in 1.16.2
}

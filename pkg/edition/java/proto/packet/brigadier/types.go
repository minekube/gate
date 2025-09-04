package brigadier

import (
	"go.minekube.com/brigodier"
)

var (
	RegistryKeyArgument brigodier.ArgumentType = &RegistryKeyArgumentType{}
	PlayerArgument      brigodier.ArgumentType = &EntityArgumentType{SingleEntity: true, OnlyPlayers: true}
)

type RegistryKeyArgumentType struct {
	Identifier string
}

func (r *RegistryKeyArgumentType) Parse(rd *brigodier.StringReader) (any, error) {
	return rd.ReadString()
}

func (r *RegistryKeyArgumentType) String() string { return "registry_key_argument" }

type ByteArgumentType byte

func (b ByteArgumentType) Parse(*brigodier.StringReader) (any, error) { return byte(0), nil }
func (b ByteArgumentType) String() string                             { return "byte" }

type IntArgumentType int

func (b IntArgumentType) Parse(*brigodier.StringReader) (any, error) { return 0, nil }
func (b IntArgumentType) String() string                             { return "int" }

var ResourceOrTagKeyArgument brigodier.ArgumentType = &ResourceOrTagKeyArgumentType{}

type ResourceOrTagKeyArgumentType RegistryKeyArgumentType

func (r *ResourceOrTagKeyArgumentType) Parse(rd *brigodier.StringReader) (any, error) {
	return rd.ReadString()
}

func (r *ResourceOrTagKeyArgumentType) String() string { return "resource_or_tag_key_argument" }

var ResourceKeyArgument brigodier.ArgumentType = &ResourceKeyArgumentType{}

type ResourceKeyArgumentType RegistryKeyArgumentType

func (r *ResourceKeyArgumentType) Parse(rd *brigodier.StringReader) (any, error) {
	return rd.ReadString()
}

func (r *ResourceKeyArgumentType) String() string { return "resource_key_argument" }

var ResourceSelectorArgument brigodier.ArgumentType = &ResourceSelectorArgumentType{}

type ResourceSelectorArgumentType RegistryKeyArgumentType

func (r *ResourceSelectorArgumentType) Parse(rd *brigodier.StringReader) (any, error) {
	return rd.ReadString()
}

func (r *ResourceSelectorArgumentType) String() string { return "resource_selector_argument" }

// EntityArgumentType represents the minecraft:entity argument type.
// See https://wiki.vg/Command_Data (minecraft:entity)
// It provides auto-substitution of online player names in commands.
type EntityArgumentType struct {
	SingleEntity bool // Only select one entity
	OnlyPlayers  bool // Only select players (not other entities)
}

func (t *EntityArgumentType) String() string { return "entity" }
func (t *EntityArgumentType) Parse(rd *brigodier.StringReader) (any, error) {
	return rd.ReadString()
}

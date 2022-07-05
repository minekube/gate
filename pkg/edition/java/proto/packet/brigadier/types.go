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

func (b ByteArgumentType) Parse(*brigodier.StringReader) (interface{}, error) { return byte(0), nil }
func (b ByteArgumentType) String() string                                     { return "byte" }

type EntityArgumentType struct {
	SingleEntity bool
	OnlyPlayers  bool
}

func (t *EntityArgumentType) String() string { return "entity" }
func (t *EntityArgumentType) Parse(rd *brigodier.StringReader) (any, error) {
	return rd.ReadString()
}

package brigadier

import "go.minekube.com/brigodier"

var (
	RegistryKeyArgument brigodier.ArgumentType = &RegistryKeyArgumentType{}
	PlayerArgument      brigodier.ArgumentType = &EntityArgumentType{SingleEntity: true, OnlyPlayers: true}
)

type RegistryKeyArgumentType struct {
	Identifier string
}

func (r *RegistryKeyArgumentType) Parse(rd *brigodier.StringReader) (interface{}, error) {
	return rd.ReadString()
}

func (r *RegistryKeyArgumentType) String() string { return "registry_key_argument" }

type EntityArgumentType struct {
	SingleEntity bool
	OnlyPlayers  bool
}

func (t *EntityArgumentType) String() string { return "entity" }
func (t *EntityArgumentType) Parse(rd *brigodier.StringReader) (interface{}, error) {
	return rd.ReadString()
}

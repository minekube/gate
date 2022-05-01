package brigadier

import "go.minekube.com/brigodier"

var (
	RegistryKeyArgument brigodier.ArgumentType = &RegistryKeyArgumentType{}
)

type RegistryKeyArgumentType struct {
	Identifier string
}

func (r *RegistryKeyArgumentType) Parse(rd *brigodier.StringReader) (interface{}, error) {
	return rd.ReadString()
}

func (r *RegistryKeyArgumentType) String() string { return "registry_key_argument" }

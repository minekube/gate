package brigadier

import (
	"go.minekube.com/brigodier"
)

var RegistryKeyArgument brigodier.ArgumentType = &RegistryKeyArgumentType{}

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

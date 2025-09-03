package configutil

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// BoolOrStruct is a type that can be either a boolean or a struct of type T.
// This is useful for configuration fields that support both:
// - Simple boolean enable/disable: `field: true`
// - Advanced struct configuration: `field: { enabled: true, option: value }`
type BoolOrStruct[T any] struct {
	isBool      bool
	boolValue   bool
	structValue T
}

// Make sure BoolOrStruct implements the interfaces at compile time.
var (
	_ yaml.Marshaler   = (*BoolOrStruct[any])(nil)
	_ yaml.Unmarshaler = (*BoolOrStruct[any])(nil)
	_ json.Marshaler   = (*BoolOrStruct[any])(nil)
	_ json.Unmarshaler = (*BoolOrStruct[any])(nil)
)

// NewBoolOrStructBool creates a BoolOrStruct with a boolean value.
func NewBoolOrStructBool[T any](value bool) BoolOrStruct[T] {
	return BoolOrStruct[T]{
		isBool:    true,
		boolValue: value,
	}
}

// NewBoolOrStructStruct creates a BoolOrStruct with a struct value.
func NewBoolOrStructStruct[T any](value T) BoolOrStruct[T] {
	return BoolOrStruct[T]{
		isBool:      false,
		structValue: value,
	}
}

// IsBool returns true if this represents a boolean value.
func (b BoolOrStruct[T]) IsBool() bool {
	return b.isBool
}

// BoolValue returns the boolean value if IsBool() is true.
func (b BoolOrStruct[T]) BoolValue() bool {
	return b.boolValue
}

// StructValue returns the struct value if IsBool() is false.
func (b BoolOrStruct[T]) StructValue() T {
	return b.structValue
}

// IsNil returns true if this BoolOrStruct is unset/nil.
func (b BoolOrStruct[T]) IsNil() bool {
	return !b.isBool && isZero(b.structValue)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (b *BoolOrStruct[T]) UnmarshalYAML(node *yaml.Node) error {
	// Handle null/nil explicitly
	if node.Tag == "!!null" {
		*b = BoolOrStruct[T]{} // Reset to zero/nil state
		return nil
	}

	// Try to unmarshal as boolean first
	var boolVal bool
	if err := node.Decode(&boolVal); err == nil {
		*b = NewBoolOrStructBool[T](boolVal)
		return nil
	}

	// Try to unmarshal as struct
	var structVal T
	if err := node.Decode(&structVal); err == nil {
		*b = NewBoolOrStructStruct(structVal)
		return nil
	}

	return fmt.Errorf("field must be either bool or struct, got %s", node.Tag)
}

// MarshalYAML implements yaml.Marshaler.
func (b BoolOrStruct[T]) MarshalYAML() (any, error) {
	if b.IsNil() {
		return nil, nil
	}
	if b.isBool {
		return b.boolValue, nil
	}
	return b.structValue, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *BoolOrStruct[T]) UnmarshalJSON(data []byte) error {
	// Handle null explicitly
	if string(data) == "null" {
		*b = BoolOrStruct[T]{} // Reset to zero/nil state
		return nil
	}

	// Try to unmarshal as boolean first
	var boolVal bool
	if err := json.Unmarshal(data, &boolVal); err == nil {
		*b = NewBoolOrStructBool[T](boolVal)
		return nil
	}

	// Try to unmarshal as struct
	var structVal T
	if err := json.Unmarshal(data, &structVal); err == nil {
		*b = NewBoolOrStructStruct(structVal)
		return nil
	}

	return fmt.Errorf("field must be either bool or struct")
}

// MarshalJSON implements json.Marshaler.
func (b BoolOrStruct[T]) MarshalJSON() ([]byte, error) {
	if b.IsNil() {
		return []byte("null"), nil
	}
	if b.isBool {
		return json.Marshal(b.boolValue)
	}
	return json.Marshal(b.structValue)
}

// String returns a string representation of the BoolOrStruct.
func (b BoolOrStruct[T]) String() string {
	if b.IsNil() {
		return "nil"
	}
	if b.isBool {
		return fmt.Sprintf("bool:%v", b.boolValue)
	}
	return fmt.Sprintf("struct:%v", b.structValue)
}

// isZero checks if a value is the zero value for its type.
func isZero[T any](value T) bool {
	var zero T
	return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", zero)
}

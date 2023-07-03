package configutil

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"gopkg.in/yaml.v3"
)

// SingleOrMulti is a type that can be either a single value or a slice of values.
type SingleOrMulti[T any] []T

// Make sure SingleOrMulti implements the interfaces at compile time.
var (
	_ yaml.Marshaler   = (*SingleOrMulti[any])(nil)
	_ yaml.Unmarshaler = (*SingleOrMulti[any])(nil)

	_ json.Marshaler   = (*SingleOrMulti[any])(nil)
	_ json.Unmarshaler = (*SingleOrMulti[any])(nil)
)

// UnmarshalYAML unmarshals the value as a YAML array if it is a slice of values.
// Otherwise, it unmarshals the single value.
func (a *SingleOrMulti[T]) UnmarshalYAML(value *yaml.Node) error {
	var multi []T
	err := value.Decode(&multi)
	if err != nil {
		var single T
		err = value.Decode(&single)
		if err != nil {
			return err
		}
		*a = []T{single}
	} else {
		*a = multi
	}
	return nil
}

// UnmarshalJSON unmarshals the value as a JSON array if it is a slice of values.
// Otherwise, it unmarshals the single value.
func (a *SingleOrMulti[T]) UnmarshalJSON(bytes []byte) error {
	var multi []T
	err := json.Unmarshal(bytes, &multi)
	if err != nil {
		var single T
		err = json.Unmarshal(bytes, &single)
		if err != nil {
			return err
		}
		*a = []T{single}
	} else {
		*a = multi
	}
	return nil
}

// MarshalYAML marshals the value as a YAML array if it is a slice of values.
// Otherwise, it marshals the single value.
func (a SingleOrMulti[T]) MarshalYAML() (any, error) {
	if a.IsMulti() {
		return a.Multi(), nil
	}
	return a.Single(), nil
}

// MarshalJSON marshals the value as a JSON array if it is a slice of values.
// Otherwise, it marshals the single value.
func (a SingleOrMulti[T]) MarshalJSON() ([]byte, error) {
	if a.IsMulti() {
		return json.Marshal(a.Multi())
	}
	return json.Marshal(a.Single())
}

// IsMulti returns true if the value is a slice of values.
func (a SingleOrMulti[T]) IsMulti() bool {
	return len(a) > 1
}

// Single returns first value in the slice or zero value if the slice is empty.
func (a SingleOrMulti[T]) Single() T {
	if len(a) == 0 {
		var zero T
		return zero
	}
	return a[0]
}

// Multi returns the slice of values.
func (a SingleOrMulti[T]) Multi() []T {
	return a
}

// Copy returns a copy of the SingleOrMulti.
func (a SingleOrMulti[T]) Copy() SingleOrMulti[T] {
	return append(SingleOrMulti[T]{}, a...)
}

// String returns the string representation of the SingleOrMulti.
func (a SingleOrMulti[T]) String() string {
	if a.IsMulti() {
		return fmt.Sprint(a.Multi())
	}
	return fmt.Sprint(a.Single())
}

// Random returns a random value from the slice or zero value if the slice is empty.
func (a SingleOrMulti[T]) Random() T {
	if len(a) == 0 {
		var zero T
		return zero
	}
	return a[rand.Intn(len(a))]
}

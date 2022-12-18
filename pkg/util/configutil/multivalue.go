package configutil

import (
	"encoding/json"
	"fmt"
	"math/rand"

	"gopkg.in/yaml.v3"
)

// SingleOrMulti is a type that can be either a single value or a slice of values.
type SingleOrMulti[T any] []T

var (
	_ yaml.Marshaler   = (*SingleOrMulti[string])(nil)
	_ yaml.Unmarshaler = (*SingleOrMulti[string])(nil)

	_ json.Marshaler   = (*SingleOrMulti[string])(nil)
	_ json.Unmarshaler = (*SingleOrMulti[string])(nil)
)

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

func (a SingleOrMulti[T]) MarshalYAML() (any, error) {
	if a.IsMulti() {
		return a.Multi(), nil
	}
	return a.Single(), nil
}

func (a SingleOrMulti[T]) MarshalJSON() ([]byte, error) {
	if a.IsMulti() {
		return json.Marshal(a.Multi())
	}
	return json.Marshal(a.Single())
}

func (a SingleOrMulti[T]) IsMulti() bool {
	return len(a) != 0
}

func (a SingleOrMulti[T]) Single() T {
	if len(a) == 0 {
		var zero T
		return zero
	}
	return a[0]
}

func (a SingleOrMulti[T]) Multi() []T {
	return a
}

func (a SingleOrMulti[T]) String() string {
	if a.IsMulti() {
		return fmt.Sprint(a.Multi())
	}
	return fmt.Sprint(a.Single())
}

func (a SingleOrMulti[T]) Random() T {
	if len(a) == 0 {
		var zero T
		return zero
	}
	return a[rand.Intn(len(a))]
}

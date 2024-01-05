package configutil

import (
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a configuration duration.
// It is a wrapper around time.Duration that implements the json.Marshaler and json.Unmarshaler interfaces.
//
//   - string is parsed using time.ParseDuration.
//   - int64 and float64 are interpreted as seconds.
type Duration time.Duration

// Make sure Duration implements the interfaces at compile time.
var (
	_ yaml.Marshaler   = (*Duration)(nil)
	_ yaml.Unmarshaler = (*Duration)(nil)

	_ json.Marshaler   = (*Duration)(nil)
	_ json.Unmarshaler = (*Duration)(nil)
)

func (d *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(*d).String())
}
func (d *Duration) UnmarshalJSON(data []byte) error {
	var a any
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	dur, err := decode(a)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d *Duration) MarshalYAML() (any, error) {
	return time.Duration(*d).String(), nil
}
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var a any
	err := value.Decode(&a)
	if err != nil {
		return err
	}
	dur, err := decode(a)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func decode(a any) (time.Duration, error) {
	switch v := a.(type) {
	case string:
		return time.ParseDuration(v)
	case float64:
		return time.Duration(v) * time.Millisecond, nil
	case int64:
		return time.Duration(v) * time.Millisecond, nil
	case int:
		return time.Duration(v) * time.Millisecond, nil
	default:
		return 0, fmt.Errorf("invalid duration type %T: %v", v, v)
	}
}

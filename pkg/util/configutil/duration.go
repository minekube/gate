package configutil

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a configuration duration.
// It is a wrapper around time.Duration that implements the json.Marshaler and json.Unmarshaler interfaces.
//
//   - string is parsed using time.ParseDuration.
//   - int64 and float64 are interpreted as seconds.
type Duration time.Duration

func (d *Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(*d).String())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var a any
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	switch v := a.(type) {
	case string:
		dur, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		*d = Duration(dur)
	case float64:
		*d = Duration(time.Duration(v) * time.Second)
	case int64:
		*d = Duration(time.Duration(v) * time.Second)
	default:
		return fmt.Errorf("invalid duration type %T: %v", v, v)
	}
	return nil
}

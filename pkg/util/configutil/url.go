package configutil

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"net/url"
)

// URL is a parsed URL.
type URL url.URL

// String returns the string representation of the URL.
func (u *URL) String() string { return u.T().String() }

// T returns the underlying url.URL.
func (u *URL) T() *url.URL {
	if u == nil {
		return nil
	}
	return (*url.URL)(u)
}

// Make sure URL implements the interfaces at compile time.
var (
	_ yaml.Marshaler   = (*URL)(nil)
	_ yaml.Unmarshaler = (*URL)(nil)

	_ json.Marshaler   = (*URL)(nil)
	_ json.Unmarshaler = (*URL)(nil)
)

func (u *URL) decode(s string) error {
	if s == "" {
		return fmt.Errorf("input URL is empty")
	}
	parsed, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("error parsing URL (%s): %w", s, err)
	}
	if parsed.String() == "" {
		return fmt.Errorf("parsed URL is empty")
	}
	*u = URL(*parsed)
	return nil
}

func (u *URL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

func (u *URL) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return u.decode(s)
}

func (u *URL) MarshalYAML() (any, error) {
	return u.String(), nil
}

func (u *URL) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	return u.decode(s)
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (u *URL) MarshalBinary() ([]byte, error) {
	return u.T().MarshalBinary()
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (u *URL) UnmarshalBinary(data []byte) error {
	return u.T().UnmarshalBinary(data)
}

package configutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/componentutil"
	"gopkg.in/yaml.v3"
)

// TextComponent is a component.Text that implements the yaml and json interfaces.
type TextComponent component.Text

// T returns the underlying component.Text.
func (t *TextComponent) T() *component.Text {
	return (*component.Text)(t)
}

// Make sure TextComponent implements the interfaces at compile time.
var (
	_ yaml.Marshaler   = (*TextComponent)(nil)
	_ yaml.Unmarshaler = (*TextComponent)(nil)

	_ json.Marshaler   = (*TextComponent)(nil)
	_ json.Unmarshaler = (*TextComponent)(nil)
)

func (t *TextComponent) MarshalYAML() (any, error) {
	j, err := t.MarshalJSON()
	return string(j), err
}

func (t *TextComponent) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	text, err := componentutil.ParseTextComponent(
		version.MaximumVersion.Protocol, s)
	if err != nil {
		return err
	}
	*t = TextComponent(*text)
	return nil
}

func (t *TextComponent) MarshalJSON() ([]byte, error) {
	b := new(bytes.Buffer)
	codec := util.JsonCodec(version.MaximumVersion.Protocol)
	err := codec.Marshal(b, t.T())
	return b.Bytes(), err
}

func (t *TextComponent) UnmarshalJSON(data []byte) error {
	text, err := componentutil.ParseTextComponent(
		version.MaximumVersion.Protocol, string(data))
	if err != nil {
		return err
	}
	*t = TextComponent(*text)
	return nil
}

// Component is a Minecraft component that implements yaml and json interfaces.
type Component struct {
	Value component.Component
}

// C returns the underlying component.
func (c *Component) C() component.Component {
	if c == nil || c.Value == nil {
		return &component.Text{}
	}
	return c.Value
}

// Make sure Component implements the interfaces at compile time.
var (
	_ yaml.Marshaler   = (*Component)(nil)
	_ yaml.Unmarshaler = (*Component)(nil)

	_ json.Marshaler   = (*Component)(nil)
	_ json.Unmarshaler = (*Component)(nil)
)

func (c *Component) MarshalYAML() (any, error) {
	if text, ok := c.C().(*component.Text); ok {
		b := new(strings.Builder)
		if err := (&legacy.Legacy{}).Marshal(b, text); err == nil {
			return b.String(), nil
		}
	}
	j, err := c.MarshalJSON()
	return string(j), err
}

func (c *Component) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := componentutil.ParseComponent(version.MaximumVersion.Protocol, s)
	if err != nil {
		return err
	}
	c.Value = parsed
	return nil
}

func (c *Component) MarshalJSON() ([]byte, error) {
	b := new(bytes.Buffer)
	codec := util.JsonCodec(version.MaximumVersion.Protocol)
	if err := codec.Marshal(b, c.C()); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (c *Component) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		c.Value = &component.Text{}
		return nil
	}
	parsed, err := componentutil.ParseComponent(version.MaximumVersion.Protocol, string(data))
	if err != nil {
		return fmt.Errorf("parse component: %w", err)
	}
	c.Value = parsed
	return nil
}

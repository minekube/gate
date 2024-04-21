package configutil

import (
	"bytes"
	"encoding/json"

	"go.minekube.com/common/minecraft/component"
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

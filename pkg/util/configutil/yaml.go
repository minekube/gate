package configutil

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

func DecodeYAMLStrict(node *yaml.Node, target any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}

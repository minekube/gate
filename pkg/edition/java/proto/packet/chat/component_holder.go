package chat

import (
	"encoding/json"
	"fmt"
	nbt2 "github.com/Tnze/go-mc/nbt"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"gopkg.in/yaml.v3"
	"io"
	"log/slog"
	"regexp"
)

func FromComponent(comp component.Component) *ComponentHolder {
	if comp == nil {
		return nil
	}
	return &ComponentHolder{Component: comp}
}

func FromComponentProtocol(comp component.Component, protocol proto.Protocol) *ComponentHolder {
	if comp == nil {
		return nil
	}
	return &ComponentHolder{Component: comp, Protocol: protocol}
}

// ComponentHolder holds a chat component that can be represented in different formats.
type ComponentHolder struct {
	Protocol  proto.Protocol
	Component component.Component
	JSON      json.RawMessage
	BinaryTag nbt2.RawMessage
}

// ReadComponentHolder reads a ComponentHolder from the provided reader.
func ReadComponentHolder(rd io.Reader, protocol proto.Protocol) (*ComponentHolder, error) {
	var c ComponentHolder
	err := c.read(rd, protocol)
	return &c, err
}

// ReadComponentHolderNP reads a ComponentHolder from the provided reader.
func ReadComponentHolderNP(rd io.Reader, protocol proto.Protocol) (ComponentHolder, error) {
	ch, err := ReadComponentHolder(rd, protocol)
	if err != nil {
		return ComponentHolder{}, err
	}
	return *ch, nil
}

// Read reads a ComponentHolder from the provided reader.
func (c *ComponentHolder) read(rd io.Reader, protocol proto.Protocol) (err error) {
	c.Protocol = protocol
	if protocol.GreaterEqual(version.Minecraft_1_20_3) {
		dec := nbt2.NewDecoder(rd)
		dec.NetworkFormat(true) // skip tag name
		_, err := dec.Decode(&c.BinaryTag)
		if err != nil {
			return fmt.Errorf("error while reading binaryTag: %w", err)
		}
		return nil
	}
	j, err := util.ReadString(rd)
	c.JSON = json.RawMessage(j)
	return err
}

// Write writes the component holder to the writer.
func (c *ComponentHolder) Write(wr io.Writer, protocol proto.Protocol) error {
	if protocol.GreaterEqual(version.Minecraft_1_20_3) {
		enc := nbt2.NewEncoder(wr)
		enc.NetworkFormat(true) // skip tag name
		err := enc.Encode(c.BinaryTag, "")
		if err != nil {
			return fmt.Errorf("error while reading binaryTag: %w", err)
		}
		return nil
	}
	j, err := c.AsJson()
	if err != nil {
		return err
	}
	return util.WriteString(wr, string(j))
}

func (c *ComponentHolder) AsComponentOrNil() component.Component {
	if c == nil {
		return nil
	}
	comp, err := c.AsComponent()
	if err != nil {
		slog.Debug("error while converting component holder to component", "error", err)
		return nil
	}
	return comp
}

// AsComponent returns the component as a component.Component.
func (c *ComponentHolder) AsComponent() (component.Component, error) {
	switch {
	case c.Component != nil:
		return c.Component, nil
	case len(c.JSON) != 0:
		var err error
		c.Component, err = util.JsonCodec(c.Protocol).Unmarshal(c.JSON)
		return c.Component, err
	case len(c.BinaryTag.Data) != 0:
		var err error
		c.JSON, err = binaryTagToJSON(&c.BinaryTag)
		if err != nil {
			return nil, fmt.Errorf("error while marshalling binaryTag to JSON: %w", err)
		}
		c.Component, err = util.JsonCodec(c.Protocol).Unmarshal(c.JSON)
		return c.Component, err
	default:
		return nil, fmt.Errorf("no component found")
	}
}

// AsJson returns the component as a JSON raw message.
func (c *ComponentHolder) AsJson() (json.RawMessage, error) {
	if len(c.JSON) != 0 {
		return c.JSON, nil
	}
	if len(c.BinaryTag.Data) != 0 {
		var err error
		c.JSON, err = binaryTagToJSON(&c.BinaryTag)
		return c.JSON, err
	}
	comp, err := c.AsComponent()
	if err != nil {
		return nil, err
	}
	c.JSON, err = util.Marshal(c.Protocol, comp)
	return c.JSON, err
}

func (c *ComponentHolder) AsJsonOrNil() json.RawMessage {
	if c == nil {
		return nil
	}
	j, err := c.AsJson()
	if err != nil {
		slog.Debug("error while converting component holder to json", "error", err)
		return nil
	}
	return j
}

// AsBinaryTag returns the component as a binary NBT tag.
//func (c *ComponentHolder) AsBinaryTag() (*nbt2.RawMessage, error) {
//	if len(c.BinaryTag.Data) != 0 {
//		return &c.BinaryTag, nil
//	}
//	j, err := c.AsJson()
//	if err != nil {
//		return nil, err
//	}
//	err = nbt.UnmarshalEncoding(j, &c.BinaryTag, nbt.BigEndian) // TODO
//	return &c.BinaryTag, err
//}

func binaryTagToJSON(tag *nbt2.RawMessage) (json.RawMessage, error) {
	return snbtToJSON(tag.String())
}

var snbtRe = regexp.MustCompile(`(?m)([^"]):([^"])`)

// snbtToJSON converts a stringified NBT to JSON.
// Example: {a:1,b:hello,c:"world",d:true} -> {"a":1,"b":"hello","c":"world","d":true}
func snbtToJSON(snbt string) (json.RawMessage, error) {
	// Add spaces after colons that are not within quotes
	snbt = snbtRe.ReplaceAllString(snbt, "$1: $2")

	// Parse non-standard json with yaml, which is a superset of json.
	// We use YAML parser, since it's a superset of JSON and quotes are optional.
	type M map[string]any
	var m M
	if err := yaml.Unmarshal([]byte(snbt), &m); err != nil {
		return nil, err
	}
	// Marshal back to JSON
	j, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return j, nil
}

package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/Tnze/go-mc/nbt"
	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/proto/nbtconv"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
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
	BinaryTag nbt.RawMessage
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
		c.BinaryTag, err = util.ReadBinaryTag(rd, protocol)
		return err
	}
	j, err := util.ReadString(rd)
	c.JSON = json.RawMessage(j)
	return err
}

// Write writes the component holder to the writer.
func (c *ComponentHolder) Write(wr io.Writer, protocol proto.Protocol) error {
	if protocol.GreaterEqual(version.Minecraft_1_20_3) {
		bt, err := c.AsBinaryTag()
		if err != nil {
			return err
		}
		return util.WriteBinaryTag(wr, protocol, bt)
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
		slog.Error("error while converting component holder to component", "error", err)
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
		c.JSON, err = nbtconv.BinaryTagToJSON(&c.BinaryTag)
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
		c.JSON, err = nbtconv.BinaryTagToJSON(&c.BinaryTag)
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
		slog.Error("error while converting component holder to json", "error", err)
		return nil
	}
	return j
}

func (c *ComponentHolder) AsBinaryTag() (util.BinaryTag, error) {
	if len(c.BinaryTag.Data) != 0 {
		return c.BinaryTag, nil
	}
	j, err := c.AsJson()
	if err != nil {
		return c.BinaryTag, err
	}
	return nbtconv.JsonToBinaryTag(j)
}

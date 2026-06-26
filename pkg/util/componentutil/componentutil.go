package componentutil

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

// ParseComponent parses a component from a string.
// The string can be either a legacy or json Minecraft text component.
func ParseComponent(protocol proto.Protocol, s string) (component.Component, error) {
	s = strings.TrimSpace(s)
	var c component.Component
	if strings.HasPrefix(s, "{") {
		var err error
		c, err = protoutil.JsonCodec(protocol).Unmarshal([]byte(s))
		if err != nil {
			return nil, err
		}
	} else {
		{
			// If the string is a json string, try to unmarshal it.
			st := strings.TrimSpace(s)
			if strings.HasPrefix(st, `"`) && strings.HasSuffix(st, `"`) {
				_ = json.Unmarshal([]byte(s), &s) // ignore error and continue
			}
		}
		var err error
		c, err = (&legacy.Legacy{}).Unmarshal([]byte(s))
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

// ParseTextComponent parses a text component from a string.
// The string can be either a legacy or json Minecraft text component.
func ParseTextComponent(protocol proto.Protocol, s string) (t *component.Text, err error) {
	c, err := ParseComponent(protocol, s)
	if err != nil {
		return nil, err
	}
	t, ok := c.(*component.Text)
	if !ok {
		return nil, fmt.Errorf("invalid text component type %T", c)
	}
	return t, nil
}

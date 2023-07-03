package componentutil

import (
	"errors"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
)

// ParseTextComponent parses a text component from a string.
// The string can be either a legacy or json Minecraft text component.
func ParseTextComponent(protocol proto.Protocol, s string) (t *component.Text, err error) {
	s = strings.TrimSpace(s)
	var c component.Component
	if strings.HasPrefix(s, "{") {
		c, err = protoutil.JsonCodec(protocol).Unmarshal([]byte(s))
	} else {
		c, err = (&legacy.Legacy{}).Unmarshal([]byte(s))
	}
	if err != nil {
		return nil, err
	}
	t, ok := c.(*component.Text)
	if !ok {
		return nil, errors.New("invalid text component")
	}
	return t, nil
}

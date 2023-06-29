package componentutil

import (
	"errors"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	protoutil "go.minekube.com/gate/pkg/edition/java/proto/util"
)

func ParseTextComponent(s string) (t *component.Text, err error) {
	var c component.Component
	if strings.HasPrefix(s, "{") {
		c, err = protoutil.LatestJsonCodec().Unmarshal([]byte(s))
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

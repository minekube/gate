package util

import (
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// JsonCodec returns the appropriate codec for the given protocol version.
// This is used to constrain messages sent to older clients.
func JsonCodec(protocol proto.Protocol) codec.Codec {
	if protocol.GreaterEqual(version.Minecraft_1_16) {
		return jsonCodec_1_16
	}
	return defaultJsonCodec
}

func LatestJsonCodec() codec.Codec {
	return jsonCodec_1_16
}

// DefaultJsonCodec returns a legacy supportive codec.
func DefaultJsonCodec() codec.Codec {
	return defaultJsonCodec
}

var (
	// Json component codec supporting pre-1.16 clients
	defaultJsonCodec = &codec.Json{}
	// Json component codec for 1.16+ clients
	jsonCodec_1_16 = &codec.Json{
		NoDownsampleColor: true,
		NoLegacyHover:     true,
	}
)

// MarshalPlain marshals a component into plain text.
// A component.Translation is formatted as "{key}".
func MarshalPlain(c component.Component) (string, error) {
	b := new(strings.Builder)
	err := marshalPlain(c, b)
	return b.String(), err
}

var plain = &codec.Plain{}

func marshalPlain(c component.Component, b *strings.Builder) error {
	switch t := c.(type) {
	case *component.Translation:
		b.WriteRune('{')
		b.WriteString(t.Key)
		b.WriteRune('}')
		for _, with := range t.With {
			if err := marshalPlain(with, b); err != nil {
				return err
			}
		}
		return nil
	default:
		return plain.Marshal(b, c)
	}
}

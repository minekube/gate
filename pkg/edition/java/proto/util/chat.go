package util

import (
	"bytes"
	"strings"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// JsonCodec returns the appropriate codec for the given protocol version.
// This is used to constrain messages sent to older clients.
func JsonCodec(protocol proto.Protocol) codec.Codec {
	if protocol.GreaterEqual(version.Minecraft_1_21_5) {
		return jsonCodec_Modern
	}
	if protocol.GreaterEqual(version.Minecraft_1_20_3) {
		return jsonCodec_Pre_1_21_5
	}
	if protocol.GreaterEqual(version.Minecraft_1_16) {
		return jsonCodec_Pre_1_20_3
	}
	return jsonCodec_Pre_1_16
}

// Marshal marshals a component into JSON.
func Marshal(protocol proto.Protocol, c component.Component) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := JsonCodec(protocol).Marshal(buf, c)
	return buf.Bytes(), err
}

func LatestJsonCodec() codec.Codec {
	return jsonCodec_Modern
}

// DefaultJsonCodec returns a legacy supportive codec.
func DefaultJsonCodec() codec.Codec {
	return jsonCodec_Pre_1_16
}

var (
	// Json component codec supporting pre-1.16 clients
	// Equivalent to PRE_1_16_SERIALIZER from Adventure
	jsonCodec_Pre_1_16 = codec.JsonPre1_16

	// Json component codec for 1.16+ clients (pre-1.20.3)
	// Equivalent to PRE_1_20_3_SERIALIZER from Adventure
	jsonCodec_Pre_1_20_3 = codec.JsonPre1_20_3

	// Json component codec for 1.20.3+ clients (pre-1.21.5)
	// Equivalent to PRE_1_21_5_SERIALIZER from Adventure
	jsonCodec_Pre_1_21_5 = codec.JsonPre1_21_5

	// Json component codec for 1.21.5+ clients (modern format)
	// Equivalent to MODERN_SERIALIZER from Adventure
	jsonCodec_Modern = codec.JsonModern
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

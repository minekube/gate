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
	if protocol.GreaterEqual(version.Minecraft_1_20_3) {
		return jsonCodec_Modern
	}
	if protocol.GreaterEqual(version.Minecraft_1_16) {
		return jsonCodec_pre_1_20_3
	}
	return jsonCodec_pre_1_16
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
	return jsonCodec_pre_1_16
}

var (
	// Json component codec supporting pre-1.16 clients
	jsonCodec_pre_1_16 = &codec.Json{
		/* TODO
		 GsonComponentSerializer.builder()
		  .downsampleColors()
		  .emitLegacyHoverEvent()
		  .legacyHoverEventSerializer(VelocityLegacyHoverEventSerializer.INSTANCE)
		  .options(
			  OptionState.optionState()
			  // before 1.16
			  .value(JSONOptions.EMIT_RGB, Boolean.FALSE)
			  .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.LEGACY_ONLY)
			  // before 1.20.3
			  .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.FALSE)
			  .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.FALSE)
			  .value(JSONOptions.VALIDATE_STRICT_EVENTS, Boolean.FALSE)
			  .build()
		  )
		  .build();
		*/
	}
	// Json component codec for 1.16+ clients
	jsonCodec_pre_1_20_3 = &codec.Json{
		NoDownsampleColor: true,
		NoLegacyHover:     true,
		/* TODO
		GsonComponentSerializer.builder()
		  .legacyHoverEventSerializer(VelocityLegacyHoverEventSerializer.INSTANCE)
		  .options(
			  OptionState.optionState()
			  // after 1.16
			  .value(JSONOptions.EMIT_RGB, Boolean.TRUE)
			  .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.MODERN_ONLY)
			  // before 1.20.3
			  .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.FALSE)
			  .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.FALSE)
			  .value(JSONOptions.VALIDATE_STRICT_EVENTS, Boolean.FALSE)
			  .build()
		  )
		  .build();
		*/
	}
	jsonCodec_Modern = &codec.Json{
		NoDownsampleColor: true,
		NoLegacyHover:     true,
		/* TODO
				GsonComponentSerializer.builder()
		          .legacyHoverEventSerializer(VelocityLegacyHoverEventSerializer.INSTANCE)
		          .options(
		              OptionState.optionState()
		              // after 1.16
		              .value(JSONOptions.EMIT_RGB, Boolean.TRUE)
		              .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.MODERN_ONLY)
		              // after 1.20.3
		              .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.TRUE)
		              .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.TRUE)
		              .value(JSONOptions.VALIDATE_STRICT_EVENTS, Boolean.TRUE)
		              .build()
		          )
		          .build();
		*/
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

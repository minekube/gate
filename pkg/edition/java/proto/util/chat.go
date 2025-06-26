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
	// Equivalent to PRE_1_16_SERIALIZER
	jsonCodec_Pre_1_16 = &codec.Json{
		UseLegacyFieldNames:          true,  // EMIT_CLICK_EVENT_TYPE = CAMEL_CASE
		UseLegacyClickEventStructure: true,  // EMIT_CLICK_EVENT_TYPE = VALUE_FIELD (universal "value")
		UseLegacyHoverEventStructure: true,  // EMIT_HOVER_EVENT_TYPE = VALUE_FIELD
		NoDownsampleColor:            false, // EMIT_RGB = FALSE (downsampleColors)
		NoLegacyHover:                false, // Support legacy hover events
		StdJson:                      true,
		/* Equivalent to PRE_1_16_SERIALIZER:
		GsonComponentSerializer.builder()
		  .downsampleColors()
		  .legacyHoverEventSerializer(NBTLegacyHoverEventSerializer.get())
		  .options(
		      OptionSchema.globalSchema().stateBuilder()
		      // before 1.16
		      .value(JSONOptions.EMIT_RGB, Boolean.FALSE)
		      .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.VALUE_FIELD)
		      .value(JSONOptions.EMIT_CLICK_EVENT_TYPE, JSONOptions.ClickEventValueMode.CAMEL_CASE)
		      // before 1.20.3
		      .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.FALSE)
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.FALSE)
		      .value(JSONOptions.VALIDATE_STRICT_EVENTS, Boolean.FALSE)
		      .build()
		  )
		  .build();
		*/
	}

	// Json component codec for 1.16+ clients (pre-1.20.3)
	// Equivalent to PRE_1_20_3_SERIALIZER
	jsonCodec_Pre_1_20_3 = &codec.Json{
		UseLegacyFieldNames:          true, // EMIT_CLICK_EVENT_TYPE = CAMEL_CASE
		UseLegacyClickEventStructure: true, // EMIT_CLICK_EVENT_TYPE = CAMEL_CASE (universal "value")
		UseLegacyHoverEventStructure: true, // EMIT_HOVER_EVENT_TYPE = CAMEL_CASE (contents wrapper)
		NoDownsampleColor:            true, // EMIT_RGB = TRUE
		NoLegacyHover:                true, // Modern hover events only
		StdJson:                      true,
		/* Equivalent to PRE_1_20_3_SERIALIZER:
		GsonComponentSerializer.builder()
		  .legacyHoverEventSerializer(NBTLegacyHoverEventSerializer.get())
		  .options(
		      OptionSchema.globalSchema().stateBuilder()
		      // after 1.16
		      .value(JSONOptions.EMIT_RGB, Boolean.TRUE)
		      .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.CAMEL_CASE)
		      .value(JSONOptions.EMIT_CLICK_EVENT_TYPE, JSONOptions.ClickEventValueMode.CAMEL_CASE)
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_KEY_AS_TYPE_AND_UUID_AS_ID, true)
		      // before 1.20.3
		      .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.FALSE)
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.FALSE)
		      .value(JSONOptions.VALIDATE_STRICT_EVENTS, Boolean.FALSE)
		      .build()
		  )
		  .build();
		*/
	}

	// Json component codec for 1.20.3+ clients (pre-1.21.5)
	// Equivalent to PRE_1_21_5_SERIALIZER
	jsonCodec_Pre_1_21_5 = &codec.Json{
		UseLegacyFieldNames:          true, // EMIT_CLICK_EVENT_TYPE = CAMEL_CASE
		UseLegacyClickEventStructure: true, // EMIT_CLICK_EVENT_TYPE = CAMEL_CASE (universal "value")
		UseLegacyHoverEventStructure: true, // EMIT_HOVER_EVENT_TYPE = CAMEL_CASE (contents wrapper)
		NoDownsampleColor:            true, // EMIT_RGB = TRUE
		NoLegacyHover:                true, // Modern hover events only
		StdJson:                      true,
		/* Equivalent to PRE_1_21_5_SERIALIZER:
		GsonComponentSerializer.builder()
		  .legacyHoverEventSerializer(NBTLegacyHoverEventSerializer.get())
		  .options(
		      OptionSchema.globalSchema().stateBuilder()
		      // after 1.16
		      .value(JSONOptions.EMIT_RGB, Boolean.TRUE)
		      .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.CAMEL_CASE)
		      .value(JSONOptions.EMIT_CLICK_EVENT_TYPE, JSONOptions.ClickEventValueMode.CAMEL_CASE)
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_KEY_AS_TYPE_AND_UUID_AS_ID, true)
		      // after 1.20.3
		      .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.TRUE)
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.TRUE)
		      .value(JSONOptions.VALIDATE_STRICT_EVENTS, Boolean.TRUE)
		      .build()
		  )
		  .build();
		*/
	}

	// Json component codec for 1.21.5+ clients (modern format)
	// Equivalent to MODERN_SERIALIZER
	jsonCodec_Modern = &codec.Json{
		UseLegacyFieldNames:          false, // EMIT_CLICK_EVENT_TYPE = SNAKE_CASE
		UseLegacyClickEventStructure: false, // EMIT_CLICK_EVENT_TYPE = SNAKE_CASE (specific fields)
		UseLegacyHoverEventStructure: false, // EMIT_HOVER_EVENT_TYPE = SNAKE_CASE (inlined structure)
		NoDownsampleColor:            true,  // EMIT_RGB = TRUE
		NoLegacyHover:                true,  // Modern hover events only
		StdJson:                      true,
		/* Equivalent to MODERN_SERIALIZER:
		GsonComponentSerializer.builder()
		  .legacyHoverEventSerializer(NBTLegacyHoverEventSerializer.get())
		  .options(
		      OptionSchema.globalSchema().stateBuilder()
		      // after 1.16
		      .value(JSONOptions.EMIT_RGB, Boolean.TRUE)
		      .value(JSONOptions.EMIT_HOVER_EVENT_TYPE, JSONOptions.HoverEventValueMode.SNAKE_CASE)
		      .value(JSONOptions.EMIT_CLICK_EVENT_TYPE, JSONOptions.ClickEventValueMode.SNAKE_CASE)
		      // after 1.20.3
		      .value(JSONOptions.EMIT_COMPACT_TEXT_COMPONENT, Boolean.TRUE)
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_ID_AS_INT_ARRAY, Boolean.TRUE)
		      // after 1.21.5
		      .value(JSONOptions.EMIT_HOVER_SHOW_ENTITY_KEY_AS_TYPE_AND_UUID_AS_ID, Boolean.FALSE)
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

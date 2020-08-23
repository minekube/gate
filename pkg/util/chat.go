package util

import (
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/gate/pkg/proto"
)

// JsonCodec returns the appropriate codec for the given protocol version.
// This is used to constrain messages sent to older clients.
func JsonCodec(protocol proto.Protocol) codec.Codec {
	if protocol.GreaterEqual(proto.Minecraft_1_16) {
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
	// Json component codec for pre-1.16 clients
	defaultJsonCodec = &codec.Json{}
	// Json component codec for 1.16+ clients
	jsonCodec_1_16 = &codec.Json{
		NoDownsampleColor: true,
		NoLegacyHover:     true,
	}
)

package packet

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"reflect"
	"testing"
)

func PacketCodings(t *testing.T, c *proto.PacketContext, samples ...proto.Packet) {
	t.Helper()

	buf := new(bytes.Buffer)
	for _, sample := range samples {
		buf.Reset()
		rType := reflect.TypeOf(sample).Elem()

		// encode sample
		assert.NoError(t, sample.Encode(c, buf), rType.String())

		// Decode into new empty clone of same type as sample
		clone := reflect.New(rType).Interface().(proto.Packet)
		assert.NoError(t, clone.Decode(c, buf), rType.String())

		// Compare sample with clone
		assert.Equal(t, sample, clone, rType.String())
		assert.Zero(t, buf.Len(), rType.String())
	}
}

func TestPackets(t *testing.T) {
	PacketCodings(t, &proto.PacketContext{
		Direction: proto.ServerBound,
		Protocol:  version.Minecraft_1_7_2.Protocol,
	},
		&Handshake{
			ProtocolVersion: int(version.Minecraft_1_7_2.Protocol),
			ServerAddress:   "localhost",
			Port:            25565,
			NextStatus:      1,
		},
		&StatusRequest{},
		&StatusResponse{Status: "TEST"},
		&StatusPing{RandomID: 1234567890},
	)
}

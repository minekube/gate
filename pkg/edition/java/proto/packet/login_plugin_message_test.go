package packet

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestLoginPluginMessage_WireFormat verifies that LoginPluginMessage.Data is encoded
// as raw remaining bytes (NOT length-prefixed), matching the Minecraft protocol spec
// and consistent with LoginPluginResponse which already uses raw bytes.
func TestLoginPluginMessage_WireFormat(t *testing.T) {
	msg := &LoginPluginMessage{
		ID:      42,
		Channel: "velocity:player_info",
		Data:    []byte{0x04}, // e.g. forwarding version 4
	}

	ctx := &proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  version.Minecraft_1_20_2.Protocol,
	}

	// Encode
	var buf bytes.Buffer
	require.NoError(t, msg.Encode(ctx, &buf))
	encoded := buf.Bytes()

	// Manually build the expected wire format:
	// VarInt(42) + String("velocity:player_info") + raw byte 0x04
	var expected bytes.Buffer
	util.PanicWriter(&expected).VarInt(42)
	util.PanicWriter(&expected).String("velocity:player_info")
	expected.WriteByte(0x04) // raw data, no length prefix
	assert.Equal(t, expected.Bytes(), encoded,
		"LoginPluginMessage data must be written as raw bytes, not length-prefixed")

	// Verify decode reads raw bytes (no length prefix)
	decoded := &LoginPluginMessage{}
	require.NoError(t, decoded.Decode(ctx, bytes.NewReader(encoded)))
	assert.Equal(t, 42, decoded.ID)
	assert.Equal(t, "velocity:player_info", decoded.Channel)
	assert.Equal(t, []byte{0x04}, decoded.Data,
		"Decoded data should be the raw remaining bytes")
}

// TestLoginPluginMessage_MultiByteData tests with larger data payloads.
func TestLoginPluginMessage_MultiByteData(t *testing.T) {
	data := []byte("hello forge handshake data")
	original := &LoginPluginMessage{
		ID:      1,
		Channel: "fml:loginwrapper",
		Data:    data,
	}

	ctx := &proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  version.Minecraft_1_20.Protocol,
	}

	var buf bytes.Buffer
	require.NoError(t, original.Encode(ctx, &buf))

	// Build expected wire format manually
	var expected bytes.Buffer
	util.PanicWriter(&expected).VarInt(1)
	util.PanicWriter(&expected).String("fml:loginwrapper")
	expected.Write(data) // raw data
	assert.Equal(t, expected.Bytes(), buf.Bytes(),
		"Multi-byte data must be written as raw bytes")

	decoded := &LoginPluginMessage{}
	require.NoError(t, decoded.Decode(ctx, bytes.NewReader(buf.Bytes())))
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.Channel, decoded.Channel)
	assert.Equal(t, data, decoded.Data)
}

// TestLoginPluginMessage_EmptyData tests with no data payload.
func TestLoginPluginMessage_EmptyData(t *testing.T) {
	original := &LoginPluginMessage{
		ID:      5,
		Channel: "test:channel",
		Data:    nil,
	}

	ctx := &proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  version.Minecraft_1_20.Protocol,
	}

	var buf bytes.Buffer
	require.NoError(t, original.Encode(ctx, &buf))

	decoded := &LoginPluginMessage{}
	require.NoError(t, decoded.Decode(ctx, &buf))

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.Channel, decoded.Channel)
	assert.Empty(t, decoded.Data)
}

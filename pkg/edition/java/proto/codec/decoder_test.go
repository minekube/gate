package codec

import (
	"bytes"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// buildStatusResponsePacket constructs a raw Minecraft status response packet.
// The packet format is: VarInt(frameLength) + VarInt(packetID=0x00) + String(status) + extraBytes
func buildStatusResponsePacket(status string, extraBytes []byte) []byte {
	var payload bytes.Buffer
	_ = util.WriteVarInt(&payload, 0x00) // StatusResponse packet ID
	_ = util.WriteString(&payload, status)
	payload.Write(extraBytes)

	var frame bytes.Buffer
	_ = util.WriteVarInt(&frame, payload.Len())
	frame.Write(payload.Bytes())
	return frame.Bytes()
}

// buildBCCExtraData constructs the extra bytes that BetterCompatibilityChecker (BCC)
// appends after the standard status response string within the same packet frame.
//
// BCC appends a second VarInt-prefixed JSON string containing mod compatibility metadata.
// This was confirmed by probing a real Fabric 1.20.1 server with BCC v4.0.1 installed.
//
// See:
//   - https://github.com/minekube/gate/issues/297
//   - https://github.com/nanite/BetterCompatibilityChecker (source repository)
//   - Config defaults "CHANGE_ME": https://github.com/nanite/BetterCompatibilityChecker/blob/main/common/src/main/java/dev/wuffs/bcc/Config.java
//   - ServerStatus CODEC modification (Fabric): https://github.com/nanite/BetterCompatibilityChecker/blob/main/fabric/src/main/java/dev/wuffs/bcc/mixin/ServerStatusMixin.java
func buildBCCExtraData(bccJSON string) []byte {
	var buf bytes.Buffer
	_ = util.WriteString(&buf, bccJSON)
	return buf.Bytes()
}

func TestDecoder_StatusResponse_NormalPacket(t *testing.T) {
	status := `{"version":{"name":"1.20.1","protocol":763},"players":{"max":20,"online":5},"description":"A Minecraft Server"}`
	raw := buildStatusResponsePacket(status, nil)

	dec := NewDecoder(bytes.NewReader(raw), proto.ClientBound, logr.Discard())
	dec.SetState(state.Status)
	dec.SetProtocol(version.Minecraft_1_20.Protocol)

	ctx, err := dec.Decode()
	require.NoError(t, err)
	require.NotNil(t, ctx)

	res, ok := ctx.Packet.(*packet.StatusResponse)
	require.True(t, ok, "expected *packet.StatusResponse, got %T", ctx.Packet)
	assert.Equal(t, status, res.Status)
}

// TestDecoder_StatusResponse_BCC tests the scenario from issue #297 where
// BetterCompatibilityChecker (BCC) appends extra mod metadata after the
// standard status response JSON within the same packet frame.
//
// BCC modifies Minecraft's ServerStatus CODEC to append a second VarInt-prefixed
// JSON string containing fields like releaseType, projectId, name, and version.
// Gate's StatusResponse.Decode only reads the first string (the standard status JSON),
// leaving BCC's extra string unread, which triggers ErrDecoderLeftBytes.
//
// The fix ensures that:
//  1. The decoder returns the decoded PacketContext even when ErrDecoderLeftBytes occurs
//     (decoder.go readPacket)
//  2. Callers like decodeStatusResponse() in lite/forward.go can ignore ErrDecoderLeftBytes
//     and use the valid status response
//
// See: https://github.com/minekube/gate/issues/297
func TestDecoder_StatusResponse_BCC(t *testing.T) {
	// Standard Minecraft status response JSON
	status := `{"version":{"name":"1.20.1","protocol":763},"description":{"text":"A Minecraft Server"},"players":{"max":20,"online":0}}`

	// BCC appends a second VarInt-prefixed JSON string with mod compatibility data.
	// These are the real defaults from BCC's Config.java ("CHANGE_ME" values):
	// https://github.com/nanite/BetterCompatibilityChecker/blob/main/common/src/main/java/dev/wuffs/bcc/Config.java
	bccJSON := `{"releaseType":"unknown","projectId":0,"name":"CHANGE_ME","version":"CHANGE_ME"}`
	extraBytes := buildBCCExtraData(bccJSON)

	raw := buildStatusResponsePacket(status, extraBytes)

	dec := NewDecoder(bytes.NewReader(raw), proto.ClientBound, logr.Discard())
	dec.SetState(state.Status)
	dec.SetProtocol(version.Minecraft_1_20.Protocol)

	ctx, err := dec.Decode()

	// The extra BCC bytes trigger ErrDecoderLeftBytes
	require.Error(t, err)
	assert.True(t, errors.Is(err, proto.ErrDecoderLeftBytes),
		"expected ErrDecoderLeftBytes, got: %v", err)

	// Critical: ctx must NOT be nil — this was the core bug.
	// Before the fix, readPacket returned (nil, ErrDecoderLeftBytes),
	// causing a nil pointer dereference in decodeStatusResponse().
	require.NotNil(t, ctx, "PacketContext must not be nil when ErrDecoderLeftBytes is returned")
	require.NotNil(t, ctx.Packet, "Packet must not be nil")

	// The status response should be correctly decoded despite the extra bytes
	res, ok := ctx.Packet.(*packet.StatusResponse)
	require.True(t, ok, "expected *packet.StatusResponse, got %T", ctx.Packet)
	assert.Equal(t, status, res.Status)
}

// TestDecoder_StatusResponse_BCC_EndToEnd tests the full error-handling flow
// as implemented in lite/forward.go decodeStatusResponse():
// ignore ErrDecoderLeftBytes but propagate any other errors.
//
// See: https://github.com/minekube/gate/issues/297
func TestDecoder_StatusResponse_BCC_EndToEnd(t *testing.T) {
	status := `{"version":{"name":"1.20.1","protocol":763},"description":{"text":"A Minecraft Server"},"players":{"max":20,"online":0}}`
	bccJSON := `{"releaseType":"unknown","projectId":0,"name":"CHANGE_ME","version":"CHANGE_ME"}`
	extraBytes := buildBCCExtraData(bccJSON)
	raw := buildStatusResponsePacket(status, extraBytes)

	dec := NewDecoder(bytes.NewReader(raw), proto.ClientBound, logr.Discard())
	dec.SetState(state.Status)
	dec.SetProtocol(version.Minecraft_1_20.Protocol)

	ctx, err := dec.Decode()

	// Simulate what decodeStatusResponse() in lite/forward.go does:
	// ignore ErrDecoderLeftBytes but propagate other errors.
	if err != nil && !errors.Is(err, proto.ErrDecoderLeftBytes) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, ctx, "PacketContext must not be nil")

	res, ok := ctx.Packet.(*packet.StatusResponse)
	require.True(t, ok)
	assert.Equal(t, status, res.Status)
}

// TestDecoder_StatusResponse_WithArbitraryExtraBytes verifies the fix works for
// any kind of extra bytes appended to a status response, not just BCC's format.
func TestDecoder_StatusResponse_WithArbitraryExtraBytes(t *testing.T) {
	status := `{"version":{"name":"1.20.1","protocol":763},"description":"Test"}`
	extraBytes := make([]byte, 256)
	for i := range extraBytes {
		extraBytes[i] = byte(i)
	}
	raw := buildStatusResponsePacket(status, extraBytes)

	dec := NewDecoder(bytes.NewReader(raw), proto.ClientBound, logr.Discard())
	dec.SetState(state.Status)
	dec.SetProtocol(version.Minecraft_1_20.Protocol)

	ctx, err := dec.Decode()

	if err != nil && !errors.Is(err, proto.ErrDecoderLeftBytes) {
		t.Fatalf("unexpected error: %v", err)
	}
	require.NotNil(t, ctx)

	res, ok := ctx.Packet.(*packet.StatusResponse)
	require.True(t, ok)
	assert.Equal(t, status, res.Status)
}

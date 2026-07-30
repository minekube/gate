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

func buildSoundEntityPacket773() (frame, payload []byte) {
	var payloadBuf bytes.Buffer
	w := util.PanicWriter(&payloadBuf)
	w.VarInt(0x72) // Clientbound Sound Entity, protocol 773
	w.VarInt(123)  // registry-backed sound; inline name/range are absent
	w.VarInt(int(packet.SoundSourcePlayer))
	w.VarInt(42)
	w.Float32(1)
	w.Float32(0.8)
	w.Int64(987654321)

	var frameBuf bytes.Buffer
	_ = util.WriteVarInt(&frameBuf, payloadBuf.Len())
	frameBuf.Write(payloadBuf.Bytes())
	return frameBuf.Bytes(), payloadBuf.Bytes()
}

// TestDecoder_SoundEntity_Protocol773_RegistryBacked verifies the decode and
// raw-forward path for the entity-attached sound emitted when a trident is
// thrown. Regression for https://github.com/minekube/gate/issues/597.
func TestDecoder_SoundEntity_Protocol773_RegistryBacked(t *testing.T) {
	frame, payload := buildSoundEntityPacket773()

	dec := NewDecoder(bytes.NewReader(frame), proto.ClientBound, logr.Discard())
	dec.SetState(state.Play)
	dec.SetProtocol(version.Minecraft_1_21_9.Protocol)

	ctx, err := dec.Decode()
	require.NoError(t, err)
	require.NotNil(t, ctx)

	sound, ok := ctx.Packet.(*packet.SoundEntityPacket)
	require.True(t, ok, "expected *packet.SoundEntityPacket, got %T", ctx.Packet)
	assert.Equal(t, 123, sound.SoundID)
	assert.Nil(t, sound.SoundName)
	assert.Nil(t, sound.FixedRange)
	assert.Equal(t, packet.SoundSourcePlayer, sound.SoundSource)
	assert.Equal(t, 42, sound.EntityID)
	assert.InDelta(t, 1, sound.Volume, 0)
	assert.InDelta(t, 0.8, sound.Pitch, 0.000001)
	assert.Equal(t, int64(987654321), sound.Seed)
	assert.Equal(t, payload, ctx.Payload)

	var forwarded bytes.Buffer
	enc := NewEncoder(&forwarded, proto.ClientBound, logr.Discard())
	enc.SetState(state.Play)
	enc.SetProtocol(version.Minecraft_1_21_9.Protocol)
	_, err = enc.Write(ctx.Payload)
	require.NoError(t, err)
	assert.Equal(t, frame, forwarded.Bytes())
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

package codec

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/proto/util"
)

// vanillaMaxFrameLength is the largest frame vanilla's Varint21FrameDecoder
// accepts: its length prefix is a VarInt of at most 21 bits, so 2^21-1 bytes.
// Spelled out literally rather than derived from MaximumFrameLength so these
// tests pin Gate to vanilla's number instead of to whatever Gate currently uses.
const vanillaMaxFrameLength = 2097151

// buildFrame builds a raw wire frame: VarInt(length) + length payload bytes.
func buildFrame(length int) []byte {
	var frame bytes.Buffer
	_ = util.WriteVarInt(&frame, length)
	frame.Write(make([]byte, length))
	return frame.Bytes()
}

func readFrame(b []byte) ([]byte, error) {
	// fullReader is what production wraps the connection in, so read the same way.
	payload, _, err := readVarIntFrame(&fullReader{bytes.NewReader(b)})
	return payload, err
}

// Gate must accept exactly the frame sizes vanilla accepts. Gate used to cap
// frames at 1MiB — half of vanilla, in the strict direction — which silently
// killed modded (Forge/NeoForge) backends whose config-phase traffic, such as
// registry sync and mod network queries, exceeds 1MiB. Gate closed the socket,
// the backend's log stayed empty, and the player got a bare "unable to connect".
//
// See https://github.com/minekube/gate/issues/930 (supersedes #587).
func TestReadVarIntFrameAcceptsVanillaMaximum(t *testing.T) {
	payload, err := readFrame(buildFrame(vanillaMaxFrameLength))
	require.NoError(t, err, "a %d byte frame is legal for vanilla and must be accepted", vanillaMaxFrameLength)
	require.Len(t, payload, vanillaMaxFrameLength)
}

func TestReadVarIntFrameRejectsOverVanillaMaximum(t *testing.T) {
	over := vanillaMaxFrameLength + 1

	_, err := readFrame(buildFrame(over))
	require.Error(t, err, "a %d byte frame exceeds vanilla's cap and must be rejected", over)

	var frameErr *FrameTooLargeError
	require.True(t, errors.As(err, &frameErr), "expected *FrameTooLargeError, got %T: %v", err, err)
	require.Equal(t, over, frameErr.Length)
	require.Equal(t, vanillaMaxFrameLength, frameErr.Max)
}

// A negative length (a VarInt with the sign bit set) must be rejected before it
// reaches make([]byte, length).
func TestReadVarIntFrameRejectsNegativeLength(t *testing.T) {
	var frame bytes.Buffer
	_ = util.WriteVarInt(&frame, -1)

	_, err := readFrame(frame.Bytes())
	require.Error(t, err)

	var frameErr *FrameTooLargeError
	require.True(t, errors.As(err, &frameErr), "expected *FrameTooLargeError, got %T: %v", err, err)
	require.Equal(t, -1, frameErr.Length)
}

// The frame cap bounds the still-compressed frame as it arrives on the wire,
// while the caps in decompress() bound the claimed decompressed size. The frame
// cap must stay the tighter of the two, otherwise the decompression caps become
// unreachable dead code rather than a second line of defense.
func TestMaximumFrameLengthBelowUncompressedCaps(t *testing.T) {
	require.Equal(t, vanillaMaxFrameLength, MaximumFrameLength, "frame cap must match vanilla's 2^21-1")
	require.Less(t, MaximumFrameLength, ServerboundUncompressedCap)
	require.Less(t, MaximumFrameLength, UncompressedCap)
}

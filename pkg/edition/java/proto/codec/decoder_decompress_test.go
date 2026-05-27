package codec

import (
	"bytes"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/gate/proto"
)

// A serverbound packet must not be allowed to claim a decompressed size larger
// than the serverbound cap, even when it is still within the larger clientbound
// cap. This limits how much memory a client can force the proxy to allocate,
// while trusted clientbound (backend->proxy) data keeps the higher limit.
func TestDecompressServerboundCapTighterThanClientbound(t *testing.T) {
	claimed := ServerboundUncompressedCap + 1 // over serverbound, under clientbound
	require.Less(t, claimed, UncompressedCap, "test assumes serverbound cap < clientbound cap")

	sb := NewDecoder(bytes.NewReader(nil), proto.ServerBound, logr.Discard())
	sb.SetCompressionThreshold(256)
	_, err := sb.decompress(claimed, bytes.NewReader(nil))
	require.Error(t, err, "serverbound decompress should reject a claim over the serverbound cap")
	require.Contains(t, err.Error(), "exceeds")

	cb := NewDecoder(bytes.NewReader(nil), proto.ClientBound, logr.Discard())
	cb.SetCompressionThreshold(256)
	_, err = cb.decompress(claimed, bytes.NewReader(nil))
	// The claim is under the clientbound cap, so it passes the size check (and
	// then fails on the empty zlib stream — a different error, not the cap).
	if err != nil {
		require.NotContains(t, err.Error(), "exceeds")
	}
}

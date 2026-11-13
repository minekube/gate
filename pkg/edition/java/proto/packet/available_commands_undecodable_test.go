package packet

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// A modded backend can put an argument-type identifier into the command tree
// that Gate's fixed vanilla table does not know (Forge and CrossStitch-style
// mods register argument types of their own). Gate does not need to understand
// the tree to forward it - it only parses it to inject proxy commands - so an
// undecodable tree must not be a session-fatal event on a proxy hop.
//
// Regression: a modded Forge 1.20.1 (protocol 763) join through the Connect edge
// died ~0.8 s after placement because the backend command tree carried an
// identifier outside the vanilla table, and the decode error closed the backend
// connection ("backend server sent a packet that could not be decoded").
func TestAvailableCommandsUndecodableTreeIsKeptVerbatim(t *testing.T) {
	str := func(s string) []byte { return append([]byte{byte(len(s))}, s...) }
	// Same shape as the operator-tree fixture, but the argument type id (99) is
	// outside the vanilla 763 table.
	want := slices.Concat(
		[]byte{4},                  // node count
		[]byte{NodeTypeRoot, 1, 1}, // root -> 1
		[]byte{NodeTypeLiteral, 1, 2}, str("modded"),
		[]byte{NodeTypeArgument | FlagExecutable, 0}, str("custom"), []byte{99},
		[]byte{NodeTypeLiteral | FlagExecutable, 0}, str("other"),
		[]byte{0}, // root index
	)
	ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_1_20.Protocol}

	var decoded AvailableCommands
	require.NoError(t, decoded.Decode(ctx, bytes.NewReader(want)),
		"an undecodable backend command tree must not fail the packet decode: the proxy hop would close the session")
	require.Nil(t, decoded.RootNode, "a tree that could not be decoded must not be presented as a decoded root node")
	require.NotEmpty(t, decoded.Raw, "the undecodable payload must be kept so it can be forwarded untouched")
	require.Error(t, decoded.DecodeErr, "the reason the tree stayed undecoded must stay observable")
	require.Equal(t, want, decoded.Raw)

	var encoded bytes.Buffer
	require.NoError(t, decoded.Encode(ctx, &encoded))
	require.Equal(t, want, encoded.Bytes(), "the client must receive the backend's command tree byte for byte")
}

// The same fail-open rule has to cover a tree that desyncs our decoder rather
// than one that only carries an unknown identifier: the read offset of the real
// failure varied between joins (8746 vs 8769 bytes), which means the tree that
// could not be decoded is dynamic. Any tree shape the proxy cannot parse is
// forwarded, not fatal.
func TestAvailableCommandsUnparsableTreeIsKeptVerbatim(t *testing.T) {
	// A node that claims a child index outside the node table: structurally
	// broken, exactly what a desync looks like from the decoder's point of view.
	want := []byte{
		2,                  // node count
		NodeTypeRoot, 1, 9, // root -> child 9 (does not exist)
		NodeTypeLiteral | FlagExecutable, 0, 'x',
		0, // root index
	}
	ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_1_20.Protocol}

	var decoded AvailableCommands
	require.NoError(t, decoded.Decode(ctx, bytes.NewReader(want)))
	require.Nil(t, decoded.RootNode)
	require.Equal(t, want, decoded.Raw)

	var encoded bytes.Buffer
	require.NoError(t, decoded.Encode(ctx, &encoded))
	require.Equal(t, want, encoded.Bytes())
}

// Vanilla trees keep the existing behaviour: they are decoded into a root node
// (so proxy commands can be injected and PlayerAvailableCommandsEvent fires) and
// re-encoded through the normal path.
func TestAvailableCommandsDecodableTreeStillDecodes(t *testing.T) {
	str := func(s string) []byte { return append([]byte{byte(len(s))}, s...) }
	want := slices.Concat(
		[]byte{3},
		[]byte{NodeTypeRoot, 1, 1},
		[]byte{NodeTypeLiteral, 1, 2}, str("pos"),
		[]byte{NodeTypeArgument | FlagExecutable, 0}, str("value"), []byte{8}, // minecraft:block_pos at 763 (no payload)
		[]byte{0},
	)
	ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_1_20.Protocol}

	var decoded AvailableCommands
	require.NoError(t, decoded.Decode(ctx, bytes.NewReader(want)))
	require.NoError(t, decoded.DecodeErr)
	require.NotNil(t, decoded.RootNode, "a vanilla tree must still be decoded")
	require.Empty(t, decoded.Raw, "a decodable tree must not fall back to byte passthrough")
	require.NoError(t, decoded.DecodeErr)

	var encoded bytes.Buffer
	require.NoError(t, decoded.Encode(ctx, &encoded))
	require.Equal(t, want, encoded.Bytes())
}

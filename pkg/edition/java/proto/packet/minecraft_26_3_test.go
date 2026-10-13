package packet

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// An operator's 26.3 command tree carries argument types whose ids were added
// or moved in 26.3. Gate must decode and re-encode such a Commands packet
// unchanged, or the player is disconnected as soon as they are opped.
func TestAvailableCommandsMinecraft263OperatorArguments(t *testing.T) {
	str := func(s string) []byte { return append([]byte{byte(len(s))}, s...) }
	want := slices.Concat(
		[]byte{5},                     // node count
		[]byte{NodeTypeRoot, 2, 1, 2}, // root -> 1, 2
		[]byte{NodeTypeLiteral, 1, 3}, str("dialog"),
		[]byte{NodeTypeLiteral, 1, 4}, str("swing"),
		[]byte{NodeTypeArgument | FlagExecutable, 0}, str("dialog"), []byte{58}, // minecraft:dialog
		[]byte{NodeTypeArgument | FlagExecutable, 0}, str("animation"), []byte{60}, // minecraft:swing_animation
		[]byte{0}, // root index
	)
	ctx := &proto.PacketContext{Direction: proto.ClientBound, Protocol: version.Minecraft_26_3.Protocol}

	var decoded AvailableCommands
	require.NoError(t, decoded.Decode(ctx, bytes.NewReader(want)))

	root := decoded.RootNode
	require.Equal(t, "minecraft:dialog", root.Literals()["dialog"].Arguments()["dialog"].Type().String())
	require.Equal(t, "minecraft:swing_animation", root.Literals()["swing"].Arguments()["animation"].Type().String())

	var encoded bytes.Buffer
	require.NoError(t, decoded.Encode(ctx, &encoded))
	require.Equal(t, want, encoded.Bytes())
}

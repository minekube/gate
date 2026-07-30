package brigadier

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

func TestMinecraft262RenamesColorArgumentToTeamColor(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, util.WriteVarInt(&buf, 16))

	arg, err := Decode(&buf, version.Minecraft_26_2.Protocol)
	require.NoError(t, err)
	require.Equal(t, "minecraft:team_color", arg.String())
}

func TestColorArgumentStillDecodesBeforeMinecraft262(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, util.WriteVarInt(&buf, 16))

	arg, err := Decode(&buf, version.Minecraft_1_21_11.Protocol)
	require.NoError(t, err)
	require.Equal(t, "minecraft:color", arg.String())
}

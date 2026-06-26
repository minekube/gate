package chat

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/nbtconv"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

func TestComponentHolderAsComponentAcceptsNBTStyleByteBooleans(t *testing.T) {
	tag, err := nbtconv.SnbtToBinaryTag(`{text:"hi",italic:0B,bold:1B}`)
	require.NoError(t, err)

	holder := &ComponentHolder{
		Protocol:  version.Minecraft_1_21_5.Protocol,
		BinaryTag: tag,
	}
	got, err := holder.AsComponent()
	require.NoError(t, err)

	text, ok := got.(*component.Text)
	require.Truef(t, ok, "got %T", got)
	require.Equal(t, "hi", text.Content)
	require.Equal(t, component.False, text.S.Italic)
	require.Equal(t, component.True, text.S.Bold)
}

func TestComponentHolderAsJsonExpandsCompactTextComponent(t *testing.T) {
	holder := &ComponentHolder{
		Protocol:  version.Minecraft_1_21_11.Protocol,
		Component: &component.Text{Content: "hi"},
	}

	got, err := holder.AsJson()
	require.NoError(t, err)
	require.JSONEq(t, `{"text":"hi"}`, string(got))
}

func TestComponentHolderAsJsonExpandsCachedCompactTextComponent(t *testing.T) {
	holder := &ComponentHolder{
		Protocol: version.Minecraft_1_21_11.Protocol,
		JSON:     []byte(`"hi"`),
	}

	got, err := holder.AsJson()
	require.NoError(t, err)
	require.JSONEq(t, `{"text":"hi"}`, string(got))
}

package gate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestWithBuiltinWasmAppendsAfterNativePluginsAndRestoresGlobal(t *testing.T) {
	previous := jproxy.Plugins
	jproxy.Plugins = []jproxy.Plugin{{Name: "application"}}
	t.Cleanup(func() { jproxy.Plugins = previous })
	expected := errors.New("stop")

	err := withBuiltinWasm(jconfig.DefaultWasm, func() error {
		require.Len(t, jproxy.Plugins, 2)
		require.Equal(t, "application", jproxy.Plugins[0].Name)
		require.Equal(t, "wasm", jproxy.Plugins[1].Name)
		return expected
	})

	require.ErrorIs(t, err, expected)
	require.Len(t, jproxy.Plugins, 1)
	require.Equal(t, "application", jproxy.Plugins[0].Name)
}

package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"

	"go.minekube.com/gate/pkg/configs"
	"go.minekube.com/gate/pkg/gate/config"
)

// shippedBuiltinCommandRefusedMessage is the default refusal as a client sees
// it: red, with the {node} placeholder (and the quotes it is wrapped in).
const shippedBuiltinCommandRefusedMessage = `§cYou don't have permission to use this command (requires "{node}").`

// renderedText is what a client receives for a component: the legacy codec
// flattening, colour codes included.
func renderedText(t *testing.T, c component.Component) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, (&legacy.Legacy{}).Marshal(&b, c))
	return b.String()
}

// TestLoadConfigBuiltinCommandRefusedMessage proves the new key reaches
// config.Config through the real loading path (config file -> viper ->
// LoadConfig), which is what an operator's config.yml goes through.
func TestLoadConfigBuiltinCommandRefusedMessage(t *testing.T) {
	load := func(t *testing.T, contents []byte) *config.Config {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.yml")
		require.NoError(t, os.WriteFile(path, contents, 0o644))
		v := viper.New()
		v.SetConfigFile(path)
		cfg, err := LoadConfig(v)
		require.NoError(t, err)
		return cfg
	}

	t.Run("configured value loads", func(t *testing.T) {
		cfg := load(t, []byte(`
config:
  bind: 127.0.0.1:25577
  builtinCommandRefusedMessage: "§eAsk an admin in #help (needs {node})"
`))
		require.NotNil(t, cfg.Config.BuiltinCommandRefusedMessage)
		require.Equal(t, "§eAsk an admin in #help (needs {node})",
			renderedText(t, cfg.Config.BuiltinCommandRefusedMessage.T()))
	})

	t.Run("omitted value keeps the built-in default", func(t *testing.T) {
		cfg := load(t, []byte(`
config:
  bind: 127.0.0.1:25577
`))
		require.NotNil(t, cfg.Config.BuiltinCommandRefusedMessage)
		require.Equal(t, shippedBuiltinCommandRefusedMessage,
			renderedText(t, cfg.Config.BuiltinCommandRefusedMessage.T()))
	})

	t.Run("shipped template carries the documented default", func(t *testing.T) {
		cfg := load(t, configs.DefaultConfigBytes)
		require.NotNil(t, cfg.Config.BuiltinCommandRefusedMessage)
		require.Equal(t, shippedBuiltinCommandRefusedMessage,
			renderedText(t, cfg.Config.BuiltinCommandRefusedMessage.T()))
	})

	// The root config.yml is the source of truth and `make sync-configs` copies
	// it into the embedded package; a change to one without the other ships a
	// template that disagrees with `gate config -t full`.
	t.Run("root and embedded templates stay identical", func(t *testing.T) {
		root, err := os.ReadFile(filepath.Join("..", "..", "config.yml"))
		require.NoError(t, err)
		embedded, err := os.ReadFile(filepath.Join("..", "configs", "config.yml"))
		require.NoError(t, err)
		require.Equal(t, string(root), string(embedded),
			"root config.yml and pkg/configs/config.yml must stay byte-identical (`make sync-configs`)")
	})
}

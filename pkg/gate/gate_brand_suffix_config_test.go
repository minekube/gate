package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/configs"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/gate/config"
)

const shippedBrandSuffix = " (Gate by Minekube)"

// TestLoadConfigBrandSuffix proves the key reaches config.Config through the real
// loading path (config file -> viper -> LoadConfig), which is what an operator's
// config.yml goes through.
func TestLoadConfigBrandSuffix(t *testing.T) {
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

	t.Run("omitted key keeps the shipped suffix", func(t *testing.T) {
		cfg := load(t, []byte(`
config:
  bind: 127.0.0.1:25577
`))
		require.NotNil(t, cfg.Config.BrandSuffix)
		require.Equal(t, shippedBrandSuffix, cfg.Config.ResolveBrandSuffix())
	})

	t.Run("empty string disables the rewrite", func(t *testing.T) {
		cfg := load(t, []byte(`
config:
  bind: 127.0.0.1:25577
  brandSuffix: ""
`))
		require.NotNil(t, cfg.Config.BrandSuffix)
		require.Equal(t, "", cfg.Config.ResolveBrandSuffix())
	})

	t.Run("custom suffix loads verbatim", func(t *testing.T) {
		cfg := load(t, []byte(`
config:
  bind: 127.0.0.1:25577
  brandSuffix: " (My Network)"
`))
		require.Equal(t, " (My Network)", cfg.Config.ResolveBrandSuffix())
	})

	t.Run("shipped template carries the documented default", func(t *testing.T) {
		cfg := load(t, configs.DefaultConfigBytes)
		require.NotNil(t, cfg.Config.BrandSuffix)
		require.Equal(t, shippedBrandSuffix, cfg.Config.ResolveBrandSuffix())
	})

	// newConfigCandidate decodes over a JSON deep copy of the defaults precisely
	// because a pointer field would otherwise be written through: yaml.v3 reuses
	// a non-nil pointer. Losing that copy would let one operator's config mutate
	// the process-wide default for every later load and reload.
	t.Run("loading does not mutate the package default", func(t *testing.T) {
		_ = load(t, []byte(`
config:
  bind: 127.0.0.1:25577
  brandSuffix: ""
`))
		require.NotNil(t, jconfig.DefaultConfig.BrandSuffix,
			"a loaded config must not write through DefaultConfig's suffix pointer")
		require.Equal(t, shippedBrandSuffix, jconfig.DefaultConfig.ResolveBrandSuffix())

		cfg := load(t, []byte(`
config:
  bind: 127.0.0.1:25577
`))
		require.Equal(t, shippedBrandSuffix, cfg.Config.ResolveBrandSuffix(),
			"a later load without the key must still see the shipped suffix")
	})
}

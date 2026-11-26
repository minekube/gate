package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/util/configutil"
	"gopkg.in/yaml.v3"
)

// shippedBrandSuffix is the suffix Gate appended before the key existed. It is
// written out literally (not via the constant) because the whole promise of the
// config key is that an unconfigured Gate keeps emitting exactly these bytes.
const shippedBrandSuffix = " (Gate by Minekube)"

func TestBrandSuffixDefaultsToShippedSuffix(t *testing.T) {
	require.Equal(t, shippedBrandSuffix, DefaultBrandSuffix)
	require.Equal(t, shippedBrandSuffix, plugin.DefaultBrandSuffix,
		"the config default and the wire-layer default must not drift")

	require.NotNil(t, DefaultConfig.BrandSuffix, "DefaultConfig must ship the suffix so the config API exposes it")
	require.Equal(t, shippedBrandSuffix, *DefaultConfig.BrandSuffix)
	require.Equal(t, shippedBrandSuffix, DefaultConfig.ResolveBrandSuffix())

	// A zero Config and a nil Config both mean "unconfigured" and must resolve
	// to the shipped suffix, never to "no branding".
	require.Equal(t, shippedBrandSuffix, (&Config{}).ResolveBrandSuffix())
	require.Equal(t, shippedBrandSuffix, (*Config)(nil).ResolveBrandSuffix())
}

// TestBrandSuffixStrictYAML proves the key is reachable through the strict
// decoder the config reload path uses (known fields on), that an omitted key
// keeps the default, and that an explicitly empty string is honoured as "no
// suffix" rather than being treated as absent.
func TestBrandSuffixStrictYAML(t *testing.T) {
	// loadedConfig mirrors pkg/gate.newConfigCandidate: the loader decodes over a
	// JSON deep copy of the defaults. The copy matters for a pointer field -
	// yaml.v3 writes through a non-nil pointer - so decoding a shallow copy
	// would mutate DefaultConfig itself. TestLoadConfigBrandSuffix in pkg/gate
	// guards the real loader against losing that deep copy.
	loadedConfig := func(t *testing.T) Config {
		t.Helper()
		encoded, err := json.Marshal(DefaultConfig)
		require.NoError(t, err)
		var cfg Config
		require.NoError(t, json.Unmarshal(encoded, &cfg))
		return cfg
	}
	decode := func(t *testing.T, doc string) Config {
		t.Helper()
		var node yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(doc), &node))
		require.NotEmpty(t, node.Content)

		cfg := loadedConfig(t)
		require.NoError(t, configutil.DecodeYAMLStrict(node.Content[0], &cfg))
		return cfg
	}

	t.Run("omitted key keeps the shipped suffix", func(t *testing.T) {
		cfg := decode(t, "bind: 127.0.0.1:25577\n")
		require.Equal(t, shippedBrandSuffix, cfg.ResolveBrandSuffix())
	})

	t.Run("empty string disables the rewrite", func(t *testing.T) {
		cfg := decode(t, "brandSuffix: \"\"\n")
		require.NotNil(t, cfg.BrandSuffix)
		require.Equal(t, "", cfg.ResolveBrandSuffix())
	})

	t.Run("custom suffix is used verbatim", func(t *testing.T) {
		cfg := decode(t, "brandSuffix: \" (My Network)\"\n")
		require.Equal(t, " (My Network)", cfg.ResolveBrandSuffix())
	})

	t.Run("unknown key is still rejected", func(t *testing.T) {
		var node yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte("brandSuffixes: []\n"), &node))
		cfg := loadedConfig(t)
		require.Error(t, configutil.DecodeYAMLStrict(node.Content[0], &cfg))
	})
}

// TestBrandSuffixJSONRoundTrip proves the key survives the JSON marshal Gate's
// HTTP config API serves, including the explicit "no suffix" value, which a
// *string field must not collapse into the absent case.
func TestBrandSuffixJSONRoundTrip(t *testing.T) {
	for _, want := range []string{shippedBrandSuffix, "", " (My Network)"} {
		cfg := DefaultConfig
		cfg.BrandSuffix = &want

		encoded, err := json.Marshal(&cfg)
		require.NoError(t, err)

		var decoded Config
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.NotNil(t, decoded.BrandSuffix)
		require.Equal(t, want, decoded.ResolveBrandSuffix())
	}

	// An absent key decodes to the unconfigured state, which resolves to the
	// shipped suffix.
	var absent Config
	require.NoError(t, json.Unmarshal([]byte(`{"bind":"127.0.0.1:25577"}`), &absent))
	require.Nil(t, absent.BrandSuffix)
	require.Equal(t, shippedBrandSuffix, absent.ResolveBrandSuffix())
}

// TestLiteWarnsOnBrandSuffix keeps the Lite-mode contract honest: Lite pipes the
// connection through unchanged, so a configured suffix never takes effect and an
// operator must be told instead of assuming it works.
func TestLiteWarnsOnBrandSuffix(t *testing.T) {
	liteConfig := func() Config {
		cfg := DefaultConfig
		cfg.Lite = liteconfig.Config{
			Enabled: true,
			Routes: []liteconfig.Route{{
				Host:    []string{"example.com"},
				Backend: []string{"127.0.0.1:25566"},
			}},
		}
		return cfg
	}

	t.Run("configured suffix warns", func(t *testing.T) {
		cfg := liteConfig()
		empty := ""
		cfg.BrandSuffix = &empty

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireWarnContains(t, warns, "Lite mode ignores config.brandSuffix")
	})

	t.Run("default suffix stays quiet", func(t *testing.T) {
		cfg := liteConfig()

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		for _, warn := range warns {
			require.NotContains(t, warn.Error(), "brandSuffix",
				"an operator who never set the key must not be warned about it")
		}
	})

	t.Run("full proxy mode never warns", func(t *testing.T) {
		cfg := DefaultConfig
		custom := " (My Network)"
		cfg.BrandSuffix = &custom

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		for _, warn := range warns {
			require.NotContains(t, warn.Error(), "brandSuffix")
		}
	})
}

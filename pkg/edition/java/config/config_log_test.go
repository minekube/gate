package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLogConfigParsesAndDefaults(t *testing.T) {
	// Opt-in: the default is console-only, so an existing config keeps its
	// behaviour byte for byte.
	require.Empty(t, DefaultConfig.Log.Directory)
	require.Equal(t, 50, DefaultConfig.Log.MaxSizeMB)
	require.Equal(t, 5, DefaultConfig.Log.MaxBackups)

	cfg := DefaultConfig
	require.NoError(t, yaml.Unmarshal([]byte(`
log:
  directory: logs
  maxSizeMB: 10
  maxBackups: 2
`), &cfg))
	require.Equal(t, "logs", cfg.Log.Directory)
	require.Equal(t, 10, cfg.Log.MaxSizeMB)
	require.Equal(t, 2, cfg.Log.MaxBackups)
	_, errs := cfg.Validate()
	require.Empty(t, errs)

	// Omitted keys keep their defaults instead of dropping to 0 and silently
	// disabling rotation.
	cfg = DefaultConfig
	require.NoError(t, yaml.Unmarshal([]byte("log:\n  directory: logs\n"), &cfg))
	require.Equal(t, 50, cfg.Log.MaxSizeMB)
	require.Equal(t, 5, cfg.Log.MaxBackups)
}

func TestLogConfigRejectsNegativeSettings(t *testing.T) {
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Log.MaxSizeMB = -1 },
		func(c *Config) { c.Log.MaxBackups = -1 },
	} {
		cfg := DefaultConfig
		mutate(&cfg)
		_, errs := cfg.Validate()
		require.NotEmpty(t, errs)
	}
}

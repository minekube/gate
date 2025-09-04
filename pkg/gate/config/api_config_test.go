package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAPIConfigFlattened(t *testing.T) {
	// Test YAML that matches our current config files (flattened structure)
	yamlConfig := `
api:
  enabled: true
  bind: "0.0.0.0:3000"
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err, "Should unmarshal flattened API config")

	assert.True(t, cfg.API.Enabled, "API should be enabled")
	assert.Equal(t, "0.0.0.0:3000", cfg.API.Config.Bind, "API bind should be parsed correctly")

	// Test validation
	warns, errs := cfg.Validate()
	assert.Empty(t, errs, "Should have no validation errors")
	assert.Empty(t, warns, "Should have no validation warnings")
}

func TestAPIConfigNested(t *testing.T) {
	// Test YAML with old nested structure (should NOT work with yaml:",inline" - this is expected)
	yamlConfig := `
api:
  enabled: true
  config:
    bind: "0.0.0.0:3000"
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err, "Should unmarshal nested API config without errors")

	assert.True(t, cfg.API.Enabled, "API should be enabled")
	assert.Equal(t, "", cfg.API.Config.Bind, "API bind should be empty (nested structure not supported with inline)")

	// Test validation (should fail because bind is empty)
	_, errs := cfg.Validate()
	assert.NotEmpty(t, errs, "Should have validation errors because nested config is ignored")
	assert.Contains(t, errs[0].Error(), "api.config.bind", "Error should mention API bind validation")
}

func TestAPIConfigDisabled(t *testing.T) {
	// Test disabled API
	yamlConfig := `
api:
  enabled: false
  bind: "localhost:8080"
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err, "Should unmarshal disabled API config")

	assert.False(t, cfg.API.Enabled, "API should be disabled")
	assert.Equal(t, "localhost:8080", cfg.API.Config.Bind, "API bind should still be parsed")

	// Test validation (should pass even when disabled)
	warns, errs := cfg.Validate()
	assert.Empty(t, errs, "Should have no validation errors for disabled API")
	assert.Empty(t, warns, "Should have no validation warnings for disabled API")
}

func TestAPIConfigInvalidBind(t *testing.T) {
	// Test invalid bind address
	yamlConfig := `
api:
  enabled: true
  bind: "invalid-address"
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err, "Should unmarshal config with invalid bind")

	assert.True(t, cfg.API.Enabled, "API should be enabled")
	assert.Equal(t, "invalid-address", cfg.API.Config.Bind, "Invalid bind should be parsed")

	// Test validation (should fail)
	_, errs := cfg.Validate()
	assert.NotEmpty(t, errs, "Should have validation errors for invalid bind")
	assert.Contains(t, errs[0].Error(), "api.config.bind", "Error should mention API bind validation")
}

func TestAPIConfigEmptyBind(t *testing.T) {
	// Test empty bind address
	yamlConfig := `
api:
  enabled: true
  bind: ""
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	require.NoError(t, err, "Should unmarshal config with empty bind")

	assert.True(t, cfg.API.Enabled, "API should be enabled")
	assert.Equal(t, "", cfg.API.Config.Bind, "Empty bind should be parsed")

	// Test validation (should fail)
	_, errs := cfg.Validate()
	assert.NotEmpty(t, errs, "Should have validation errors for empty bind")
	assert.Contains(t, errs[0].Error(), "api.config.bind", "Error should mention API bind validation")
}
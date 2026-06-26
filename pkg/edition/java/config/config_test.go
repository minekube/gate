package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"gopkg.in/yaml.v3"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
)

func Test_texts(t *testing.T) {
	require.NotNil(t, defaultMotd())
	require.NotNil(t, defaultShutdownReason())
}

func TestStatusMotdAcceptsObjectRootComponent(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(`
status:
  motd: '{"fallback":"diamond","sprite":"minecraft:item/diamond"}'
`), &cfg))

	require.IsType(t, &component.Object{}, cfg.Status.Motd.C())
}

func TestViaConfigValidate(t *testing.T) {
	cfg := DefaultConfig
	cfg.Servers = map[string]string{"Lobby": "127.0.0.1:25566"}
	cfg.Try = []string{"Lobby"}
	cfg.Via = Via{
		Enabled: true,
		Mode:    "embedded",
	}

	_, errs := cfg.Validate()
	require.Empty(t, errs)
}

func TestViaConfigHasNoBackendOverrideSetting(t *testing.T) {
	typ := reflect.TypeOf(Via{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		yamlName, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		require.NotEqual(t, "backends", yamlName, "via config should stay automatic and not expose per-backend overrides")
	}
}

func TestViaConfigRejectsInvalidMode(t *testing.T) {
	cfg := DefaultConfig
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:25566"}
	cfg.Try = []string{"lobby"}
	cfg.Via = Via{
		Enabled: true,
		Mode:    "native",
	}

	_, errs := cfg.Validate()
	require.NotEmpty(t, errs)
}

func TestViaConfigRejectsInvalidBind(t *testing.T) {
	cfg := DefaultConfig
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:25566"}
	cfg.Try = []string{"lobby"}
	cfg.Via = Via{
		Enabled: true,
		Bind:    "127.0.0.1",
	}

	_, errs := cfg.Validate()
	require.NotEmpty(t, errs)
}

func TestViaConfigIgnoredInLiteMode(t *testing.T) {
	cfg := DefaultConfig
	cfg.Lite = liteconfig.Config{
		Enabled: true,
		Routes: []liteconfig.Route{{
			Host:    []string{"example.com"},
			Backend: []string{"127.0.0.1:25566"},
		}},
	}
	cfg.Via = Via{
		Enabled: true,
	}

	_, errs := cfg.Validate()
	require.Empty(t, errs)
}

func TestBedrockConfig_ManagedShorthand(t *testing.T) {
	yamlConfig := `
bedrock:
  managed: true
`

	type testConfig struct {
		Bedrock bconfig.BedrockConfig `yaml:"bedrock"`
	}

	var cfg testConfig
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Test that managed: true shorthand sets enabled: true
	if !cfg.Bedrock.Enabled {
		t.Error("Expected Bedrock.Enabled to be true when managed: true")
	}

	// Test that managed field is properly set
	if cfg.Bedrock.Managed.IsNil() {
		t.Fatal("Expected Bedrock.Managed to be set")
	}

	if !cfg.Bedrock.Managed.IsBool() || !cfg.Bedrock.Managed.BoolValue() {
		t.Errorf("Expected Bedrock.Managed to be true, got %v", cfg.Bedrock.Managed.BoolValue())
	}
}

func TestBedrockConfig_TopLevelBoolEnablesManagedGeyserlite(t *testing.T) {
	yamlConfig := `
bedrock: true
`

	type testConfig struct {
		Bedrock bconfig.BedrockConfig `yaml:"bedrock"`
	}

	var cfg testConfig
	if err := yaml.Unmarshal([]byte(yamlConfig), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if !cfg.Bedrock.Enabled {
		t.Fatal("Expected Bedrock.Enabled to be true when bedrock: true")
	}
	if cfg.Bedrock.Managed.IsNil() {
		t.Fatal("Expected Bedrock.Managed to be set when bedrock: true")
	}
	if !cfg.Bedrock.Managed.IsBool() || !cfg.Bedrock.Managed.BoolValue() {
		t.Fatalf("Expected Bedrock.Managed to be true, got %v", cfg.Bedrock.Managed)
	}

	bedrockConfig := cfg.Bedrock.ToConfig()
	managedConfig := bedrockConfig.GetManaged()
	if !managedConfig.Enabled {
		t.Fatal("Expected resolved managed config to be enabled")
	}
	if managedConfig.Engine != bconfig.ManagedEngineGeyserlite {
		t.Fatalf("Expected bedrock: true to default to geyserlite engine, got %q", managedConfig.Engine)
	}
}

func TestBedrockConfig_ManagedJavaEngine(t *testing.T) {
	yamlConfig := `
bedrock:
  managed:
    enabled: true
    engine: java
`

	type testConfig struct {
		Bedrock bconfig.BedrockConfig `yaml:"bedrock"`
	}

	var cfg testConfig
	if err := yaml.Unmarshal([]byte(yamlConfig), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	bedrockConfig := cfg.Bedrock.ToConfig()
	managedConfig := bedrockConfig.GetManaged()
	if managedConfig.Engine != bconfig.ManagedEngineJava {
		t.Fatalf("Expected managed engine java, got %q", managedConfig.Engine)
	}
}

func TestBedrockConfig_ManagedNestedEngineConfig(t *testing.T) {
	yamlConfig := `
bedrock:
  managed:
    enabled: true
    engine: geyserlite
    geyserlite:
      mode: embedded
      version: v0.2.1
    java:
      dataDir: /srv/geyser
      autoUpdate: false
`

	type testConfig struct {
		Bedrock bconfig.BedrockConfig `yaml:"bedrock"`
	}

	var cfg testConfig
	if err := yaml.Unmarshal([]byte(yamlConfig), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	bedrockConfig := cfg.Bedrock.ToConfig()
	managedConfig := bedrockConfig.GetManaged()
	if managedConfig.Engine != bconfig.ManagedEngineGeyserlite {
		t.Fatalf("Expected managed engine geyserlite, got %q", managedConfig.Engine)
	}
	if managedConfig.Mode != "embedded" {
		t.Fatalf("Expected geyserlite mode embedded, got %q", managedConfig.Mode)
	}
	if managedConfig.Version != "v0.2.1" {
		t.Fatalf("Expected geyserlite version v0.2.1, got %q", managedConfig.Version)
	}
	if managedConfig.DataDir != bconfig.DefaultManaged.DataDir {
		t.Fatalf("Expected inactive java dataDir to be ignored, got %q", managedConfig.DataDir)
	}
	if !managedConfig.AutoUpdate {
		t.Fatal("Expected inactive java autoUpdate false to be ignored")
	}
}

func TestBedrockConfig_FlattenedStructure(t *testing.T) {
	yamlConfig := `
bedrock:
  enabled: true
  geyserListenAddr: "localhost:25567"
  usernameFormat: ".[%s]"
  floodgateKeyPath: "custom-key.pem"
  managed:
    enabled: true
    autoUpdate: false
    configOverrides:
      debug-mode: true
      max-players: 200
      bedrock:
        port: 19133
`

	type testConfig struct {
		Bedrock bconfig.BedrockConfig `yaml:"bedrock"`
	}

	var cfg testConfig
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Test flattened fields
	if !cfg.Bedrock.Enabled {
		t.Error("Expected Bedrock.Enabled to be true")
	}
	if cfg.Bedrock.GeyserListenAddr != "localhost:25567" {
		t.Errorf("Expected GeyserListenAddr to be 'localhost:25567', got %s", cfg.Bedrock.GeyserListenAddr)
	}
	if cfg.Bedrock.UsernameFormat != ".[%s]" {
		t.Errorf("Expected UsernameFormat to be '.[%%s]', got %s", cfg.Bedrock.UsernameFormat)
	}
	if cfg.Bedrock.FloodgateKeyPath != "custom-key.pem" {
		t.Errorf("Expected FloodgateKeyPath to be 'custom-key.pem', got %s", cfg.Bedrock.FloodgateKeyPath)
	}

	// Test managed struct
	if cfg.Bedrock.Managed.IsNil() {
		t.Fatal("Expected Bedrock.Managed to be set")
	}

	if cfg.Bedrock.Managed.IsBool() {
		t.Fatal("Expected Bedrock.Managed to be a struct, not bool")
	}

	managedStruct := cfg.Bedrock.Managed.StructValue()

	if !managedStruct.Enabled {
		t.Error("Expected managed.enabled to be true")
	}

	// Check bedrock port is in configOverrides (type-safe access)
	if managedStruct.ConfigOverrides == nil {
		t.Fatal("Expected configOverrides to be set")
	}
	if bedrockConfig, ok := managedStruct.ConfigOverrides["bedrock"].(map[string]any); ok {
		if port, ok := bedrockConfig["port"].(int); !ok || port != 19133 {
			t.Errorf("Expected configOverrides.bedrock.port to be 19133, got %v", bedrockConfig["port"])
		}
	} else {
		t.Error("Expected configOverrides.bedrock to be set")
	}

	if managedStruct.AutoUpdate {
		t.Error("Expected managed.autoUpdate to be false")
	}
}

func TestBedrockConfig_IntegrationTest(t *testing.T) {
	// Full integration test: YAML -> BedrockConfig -> Config
	yamlConfig := `
bedrock:
  managed: true
  usernameFormat: ".[%s]"
  floodgateKeyPath: "test-key.pem"
`

	type testConfig struct {
		Bedrock bconfig.BedrockConfig `yaml:"bedrock"`
	}

	var cfg testConfig
	err := yaml.Unmarshal([]byte(yamlConfig), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Test the full conversion chain
	bedrockConfig := cfg.Bedrock.ToConfig()
	managedConfig := bedrockConfig.GetManaged()

	// Verify the managed: true shorthand worked
	if !cfg.Bedrock.Enabled {
		t.Error("Expected Bedrock.Enabled to be true from managed: true shorthand")
	}
	if !managedConfig.Enabled {
		t.Error("Expected resolved managed config to be enabled")
	}

	// Verify custom fields were preserved
	if bedrockConfig.UsernameFormat != ".[%s]" {
		t.Errorf("Expected UsernameFormat to be preserved as '.[%%s]', got %s", bedrockConfig.UsernameFormat)
	}
	if bedrockConfig.FloodgateKeyPath != "test-key.pem" {
		t.Errorf("Expected FloodgateKeyPath to be preserved as 'test-key.pem', got %s", bedrockConfig.FloodgateKeyPath)
	}

	// Verify defaults were applied where needed
	if bedrockConfig.GeyserListenAddr != bconfig.DefaultConfig.GeyserListenAddr {
		t.Errorf("Expected default GeyserListenAddr to be applied")
	}
}

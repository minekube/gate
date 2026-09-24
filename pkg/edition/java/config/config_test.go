package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"gopkg.in/yaml.v3"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/configutil"
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

func TestOfflineModeUsernameBlacklistValidation(t *testing.T) {
	t.Run("accepts distinct Minecraft usernames", func(t *testing.T) {
		cfg := DefaultConfig
		cfg.OfflineModeUsernameBlacklist = []string{"AdminName", "Owner_2"}

		_, errs := cfg.Validate()
		require.Empty(t, errs)
	})

	t.Run("rejects invalid and case-insensitive duplicate usernames", func(t *testing.T) {
		cfg := DefaultConfig
		cfg.OfflineModeUsernameBlacklist = []string{"AdminName", "adminname", "not valid"}

		_, errs := cfg.Validate()
		require.Len(t, errs, 2)
		require.Contains(t, errs[0].Error(), "case-insensitively identical")
		require.Contains(t, errs[1].Error(), "2-16 character Minecraft username")
	})

	t.Run("accepts all and connect scopes, rejects other values", func(t *testing.T) {
		cfg := DefaultConfig
		for _, scope := range []OfflineModeUsernameBlacklistScope{
			OfflineModeUsernameBlacklistScopeAll,
			OfflineModeUsernameBlacklistScopeConnect,
			"", // omitted in older configurations means all
		} {
			cfg.OfflineModeUsernameBlacklistScope = scope
			_, errs := cfg.Validate()
			require.Empty(t, errs, "scope %q", scope)
		}

		cfg.OfflineModeUsernameBlacklistScope = "tunnel"
		_, errs := cfg.Validate()
		requireErrorContains(t, errs, "Invalid offlineModeUsernameBlacklistScope")
	})
}

// TestLiteIgnoredSettingsWarn covers https://github.com/minekube/gate/issues/929: Lite mode
// pipes the connection through unchanged, so full proxy settings are inert and Gate must say
// so instead of silently accepting them.
func TestLiteIgnoredSettingsWarn(t *testing.T) {
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

	t.Run("velocity forwarding warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.Forwarding.Mode = VelocityForwardingMode
		cfg.Forwarding.VelocitySecret = "secret"

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireWarnContains(t, warns, "Lite mode ignores player info forwarding")
		requireWarnContains(t, warns, `"you need to be running velocity, or a velocity proxy with modern forwarding"`)
	})

	t.Run("bungeeguard forwarding warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.Forwarding.Mode = BungeeGuardForwardingMode
		cfg.Forwarding.BungeeGuardSecret = "secret"

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores player info forwarding")
	})

	t.Run("unknown forwarding mode warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.Forwarding.Mode = "modern"

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores player info forwarding")
	})

	t.Run("velocity secret alone warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.Forwarding.VelocitySecret = "secret"

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores player info forwarding")
	})

	t.Run("servers try and forcedHosts warn", func(t *testing.T) {
		cfg := liteConfig()
		cfg.Servers = map[string]string{"lobby": "127.0.0.1:25566"}
		cfg.Try = []string{"lobby"}
		cfg.ForcedHosts = ForcedHosts{"example.com": []string{"lobby"}}

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores servers, try and forcedHosts")
	})

	t.Run("offline mode warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.OnlineMode = false

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores onlineMode")
	})

	t.Run("compression warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.Compression.Threshold = 0

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores compression")
	})

	t.Run("announceForge warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.AnnounceForge = true

		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "Lite mode ignores announceForge")
	})

	t.Run("offline username blacklist warns", func(t *testing.T) {
		cfg := liteConfig()
		cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireWarnContains(t, warns, "Lite mode ignores offlineModeUsernameBlacklist")
	})

	t.Run("lite defaults do not warn", func(t *testing.T) {
		cfg := liteConfig()

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireNoWarnContains(t, warns, "Lite mode ignores")
	})

	t.Run("full mode is unaffected", func(t *testing.T) {
		cfg := DefaultConfig
		cfg.Forwarding.Mode = VelocityForwardingMode
		cfg.Forwarding.VelocitySecret = "secret"
		cfg.Servers = map[string]string{"lobby": "127.0.0.1:25566"}
		cfg.Try = []string{"lobby"}
		cfg.ForcedHosts = ForcedHosts{"example.com": []string{"lobby"}}
		cfg.AnnounceForge = true
		cfg.OnlineMode = false

		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireNoWarnContains(t, warns, "Lite mode ignores")
	})
}

// TestAuthPrivateKeyBitsStrictYAML proves the login key size is reachable from
// the config file through the same strict decoder the loader uses
// (known-fields on), so an unknown field cannot silently swallow it.
func TestAuthPrivateKeyBitsStrictYAML(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("auth:\n  privateKeyBits: 2048\n"), &node))
	require.NotEmpty(t, node.Content)

	var cfg Config
	require.NoError(t, configutil.DecodeYAMLStrict(node.Content[0], &cfg))
	require.Equal(t, 2048, cfg.Auth.PrivateKeyBits)

	// Unset stays unset: 0 means "use the built-in default".
	var emptyNode yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("auth: {}\n"), &emptyNode))
	var unset Config
	require.NoError(t, configutil.DecodeYAMLStrict(emptyNode.Content[0], &unset))
	require.Zero(t, unset.Auth.PrivateKeyBits)
}

// TestAuthPrivateKeyBitsValidate pins the accepted range of
// auth.privateKeyBits: unset (0) means the built-in default, an explicit size
// must be one crypto/rsa can generate (>= 1024 bits), and the upper bound is
// bounded so a typo cannot stall startup with an enormous key.
func TestAuthPrivateKeyBitsValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		bits    int
		wantErr string
	}{
		{name: "unset uses the default"},
		{name: "1024 is the vanilla size", bits: 1024},
		{name: "2048", bits: 2048},
		{name: "3072", bits: 3072},
		{name: "8192 is the upper bound", bits: 8192},
		{name: "below the rsa minimum", bits: 512, wantErr: "privateKeyBits"},
		{name: "negative", bits: -2048, wantErr: "privateKeyBits"},
		{name: "absurdly large", bits: 65536, wantErr: "privateKeyBits"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig
			cfg.Bind = "127.0.0.1:25577"
			cfg.Auth.PrivateKeyBits = tt.bits

			_, errs := cfg.Validate()
			if tt.wantErr == "" {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errsContaining(errs, tt.wantErr), 1)
		})
	}
}

// TestAuthPrivateKeyBitsIgnoredByLite proves the key size is not another
// silently inert setting in Lite mode, which never takes part in login.
func TestAuthPrivateKeyBitsIgnoredByLite(t *testing.T) {
	cfg := DefaultConfig
	cfg.Lite = liteconfig.Config{
		Enabled: true,
		Routes: []liteconfig.Route{{
			Host:    []string{"example.com"},
			Backend: []string{"127.0.0.1:25566"},
		}},
	}
	cfg.Auth.PrivateKeyBits = 2048

	warns, errs := cfg.Validate()
	require.Empty(t, errs)
	requireWarnContains(t, warns, "Lite mode ignores auth.privateKeyBits")
}

func requireWarnContains(t *testing.T, warns []error, want string) {
	t.Helper()
	for _, warn := range warns {
		if strings.Contains(warn.Error(), want) {
			return
		}
	}
	t.Fatalf("expected warning containing %q, got %v", want, warns)
}

func requireNoWarnContains(t *testing.T, warns []error, unwanted string) {
	t.Helper()
	for _, warn := range warns {
		if strings.Contains(warn.Error(), unwanted) {
			t.Fatalf("unexpected warning containing %q: %v", unwanted, warn)
		}
	}
}

func TestBackendFloodgateValidation(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		cfg := DefaultConfig
		cfg.Forwarding.Mode = NoneForwardingMode
		cfg.Servers = map[string]string{"lobby": "127.0.0.1:25566"}
		cfg.Try = []string{"lobby"}

		_, errs := cfg.Validate()
		require.Empty(t, errs)
	})

	t.Run("enabled requires bedrock", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Bedrock.Enabled = false

		_, errs := cfg.Validate()
		requireErrorContains(t, errs, "bedrock.backendFloodgate requires bedrock.enabled")
	})

	t.Run("enabled requires bedrock in lite mode", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Lite = liteconfig.Config{
			Enabled: true,
			Routes: []liteconfig.Route{{
				Host:    []string{"example.com"},
				Backend: []string{"127.0.0.1:25566"},
			}},
		}
		cfg.Bedrock.Enabled = false

		_, errs := cfg.Validate()
		requireErrorContains(t, errs, "bedrock.backendFloodgate requires bedrock.enabled")
	})

	t.Run("enabled requires allowed servers", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Bedrock.BackendFloodgate.AllowedServers = nil

		_, errs := cfg.Validate()
		requireErrorContains(t, errs, "bedrock.backendFloodgate.allowedServers must not be empty")
	})

	t.Run("allowed servers must exist", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Bedrock.BackendFloodgate.AllowedServers = []string{"missing"}

		_, errs := cfg.Validate()
		requireErrorContains(t, errs, `bedrock.backendFloodgate.allowedServers server "missing" must be registered under servers`)
	})

	t.Run("allowed servers are case-normalized", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Bedrock.BackendFloodgate.AllowedServers = []string{"Lobby"}

		_, errs := cfg.Validate()
		require.Empty(t, errs)
	})

	t.Run("enabled requires floodgate key unless managed can generate it", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Bedrock.FloodgateKeyPath = filepath.Join(t.TempDir(), "missing.key")

		_, errs := cfg.Validate()
		requireErrorContains(t, errs, "bedrock.backendFloodgate requires readable floodgateKeyPath")

		cfg.Bedrock.Managed = bconfig.BoolOrManagedGeyser{}
		cfg.Bedrock.Managed = configutil.NewBoolOrStructBool[bconfig.ManagedGeyser](true)
		_, errs = cfg.Validate()
		require.Empty(t, errs)
	})

	t.Run("forwarding mode compatibility", func(t *testing.T) {
		for _, mode := range []ForwardingMode{NoneForwardingMode, VelocityForwardingMode} {
			cfg := validBackendFloodgateConfig(t)
			cfg.Forwarding.Mode = mode
			_, errs := cfg.Validate()
			require.Empty(t, errs, "mode %q should be allowed", mode)
		}

		for _, mode := range []ForwardingMode{LegacyForwardingMode, BungeeGuardForwardingMode} {
			cfg := validBackendFloodgateConfig(t)
			cfg.Forwarding.Mode = mode
			_, errs := cfg.Validate()
			requireErrorContains(t, errs, "bedrock.backendFloodgate is incompatible with forwarding.mode")
		}
	})

	t.Run("unknown forwarding mode is rejected in lite mode", func(t *testing.T) {
		cfg := validBackendFloodgateConfig(t)
		cfg.Lite = liteconfig.Config{
			Enabled: true,
			Routes: []liteconfig.Route{{
				Host:    []string{"example.com"},
				Backend: []string{"127.0.0.1:25566"},
			}},
		}
		cfg.Forwarding.Mode = "typo"

		_, errs := cfg.Validate()
		requireErrorContains(t, errs, "bedrock.backendFloodgate requires forwarding.mode none or velocity")
	})
}

func validBackendFloodgateConfig(t *testing.T) Config {
	t.Helper()

	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	require.NoError(t, os.WriteFile(keyPath, []byte("0123456789abcdef"), 0o600))

	cfg := DefaultConfig
	cfg.Forwarding.Mode = NoneForwardingMode
	cfg.Servers = map[string]string{
		"lobby":    "127.0.0.1:25566",
		"survival": "127.0.0.1:25567",
	}
	cfg.Try = []string{"lobby"}
	cfg.Bedrock.Enabled = true
	cfg.Bedrock.FloodgateKeyPath = keyPath
	cfg.Bedrock.BackendFloodgate = bconfig.BackendFloodgate{
		Enabled:        true,
		AllowedServers: []string{"lobby"},
	}
	return cfg
}

func requireErrorContains(t *testing.T, errs []error, want string) {
	t.Helper()
	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return
		}
	}
	t.Fatalf("expected error containing %q, got %v", want, errs)
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

func TestProxyProtocolTrustedProxiesValidate(t *testing.T) {
	base := func() Config {
		cfg := DefaultConfig
		cfg.Servers = map[string]string{"Lobby": "127.0.0.1:25566"}
		cfg.Try = []string{"Lobby"}
		return cfg
	}

	t.Run("defaults are valid", func(t *testing.T) {
		cfg := base()
		cfg.ProxyProtocol = true
		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		require.Empty(t, warnsContaining(warns, "proxyProtocolTrustedProxies"))
	})

	t.Run("malformed entries are rejected even while disabled", func(t *testing.T) {
		cfg := base()
		cfg.ProxyProtocolTrustedProxies = []string{"10.0.0.0/8", "not-an-ip"}
		_, errs := cfg.Validate()
		require.Len(t, errsContaining(errs, "proxyProtocolTrustedProxies"), 1)
	})

	t.Run("trusting every upstream warns", func(t *testing.T) {
		cfg := base()
		cfg.ProxyProtocol = true
		cfg.ProxyProtocolTrustedProxies = []string{"0.0.0.0/0"}
		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		require.Len(t, warnsContaining(warns, "proxyProtocolTrustedProxies"), 1)
	})

	t.Run("trusting every upstream while disabled does not warn", func(t *testing.T) {
		cfg := base()
		cfg.ProxyProtocolTrustedProxies = []string{"::/0"}
		warns, _ := cfg.Validate()
		require.Empty(t, warnsContaining(warns, "proxyProtocolTrustedProxies"))
	})
}

func TestResolveProxyProtocolTrustedProxies(t *testing.T) {
	require.Equal(t, DefaultProxyProtocolTrustedProxies(), ResolveProxyProtocolTrustedProxies(nil))
	require.Equal(t, DefaultProxyProtocolTrustedProxies(), ResolveProxyProtocolTrustedProxies([]string{}))
	require.Equal(t, []string{"10.0.0.0/8"}, ResolveProxyProtocolTrustedProxies([]string{"10.0.0.0/8"}))

	// The defaults must never be shared, so a caller cannot widen them for everyone.
	defaults := DefaultProxyProtocolTrustedProxies()
	defaults[0] = "0.0.0.0/0"
	require.NotContains(t, DefaultProxyProtocolTrustedProxies(), "0.0.0.0/0")
}

func warnsContaining(warns []error, substr string) []error { return errsContaining(warns, substr) }

func errsContaining(errs []error, substr string) []error {
	var found []error
	for _, err := range errs {
		if strings.Contains(err.Error(), substr) {
			found = append(found, err)
		}
	}
	return found
}

func TestValidatePremiumProtection(t *testing.T) {
	validIdentityStore := func() Config {
		cfg := DefaultConfig
		cfg.IdentityStore.Enabled = true
		cfg.IdentityStore.Path = "identities.db"
		return cfg
	}

	t.Run("list requires names", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.PremiumProtection.Mode = PremiumProtectionList
		_, errs := cfg.Validate()
		require.Len(t, errsContaining(errs, "premiumProtection.names must not be empty"), 1)
	})

	t.Run("unknown mode is an error", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.PremiumProtection.Mode = "bogus"
		_, errs := cfg.Validate()
		require.Len(t, errsContaining(errs, "premiumProtection.mode"), 1)
	})

	t.Run("invalid name entry is an error", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.PremiumProtection.Mode = PremiumProtectionList
		cfg.IdentityStore.PremiumProtection.Names = []string{"not a name!"}
		_, errs := cfg.Validate()
		require.Len(t, errsContaining(errs, "premiumProtection.names entry"), 1)
	})

	t.Run("uuid entries are accepted", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.PremiumProtection.Mode = PremiumProtectionList
		cfg.IdentityStore.PremiumProtection.Names = []string{"8707e474-7b5c-4d02-bba7-e577504c7656"}
		_, errs := cfg.Validate()
		require.Empty(t, errsContaining(errs, "premiumProtection"))
	})

	t.Run("names are ignored while the mode is none", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.PremiumProtection.Names = []string{"Steve"}
		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireWarnContains(t, warns, "premiumProtection.names is ignored while the mode is none")
	})

	t.Run("deprecated alias warns and still applies", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.ProtectPremiumAccounts = true
		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireWarnContains(t, warns, "protectPremiumAccounts is deprecated")
	})

	t.Run("mode wins over the deprecated alias", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.IdentityStore.ProtectPremiumAccounts = true
		cfg.IdentityStore.PremiumProtection.Mode = PremiumProtectionList
		cfg.IdentityStore.PremiumProtection.Names = []string{"Steve"}
		warns, errs := cfg.Validate()
		require.Empty(t, errs)
		requireWarnContains(t, warns, "protectPremiumAccounts is ignored")
	})

	t.Run("offline mode cannot authenticate protected accounts", func(t *testing.T) {
		cfg := validIdentityStore()
		cfg.OnlineMode = false
		cfg.IdentityStore.PremiumProtection.Mode = PremiumProtectionAll
		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "needs onlineMode: true to work")
	})

	t.Run("protection without the store warns", func(t *testing.T) {
		cfg := DefaultConfig
		cfg.IdentityStore.PremiumProtection.Mode = PremiumProtectionAll
		warns, _ := cfg.Validate()
		requireWarnContains(t, warns, "while identityStore.enabled is false")
	})
}

// The mode has exactly three values. The boolean spellings such a field invites
// (off, false, no, 0) are configuration errors rather than synonyms, while an
// unset mode means none and the spelling of a valid mode may vary in case and
// surrounding space.
func TestPremiumProtectionModeSpellings(t *testing.T) {
	cfgWithMode := func(mode PremiumProtectionMode) Config {
		cfg := DefaultConfig
		cfg.IdentityStore.Enabled = true
		cfg.IdentityStore.Path = "identities.db"
		cfg.IdentityStore.PremiumProtection.Mode = mode
		cfg.IdentityStore.PremiumProtection.Names = []string{"Steve"}
		return cfg
	}

	t.Run("valid modes normalize", func(t *testing.T) {
		for written, want := range map[string]PremiumProtectionMode{
			"":       PremiumProtectionNone,
			"none":   PremiumProtectionNone,
			"list":   PremiumProtectionList,
			"all":    PremiumProtectionAll,
			" none ": PremiumProtectionNone,
			"ALL":    PremiumProtectionAll,
			"List":   PremiumProtectionList,
		} {
			cfg := cfgWithMode(PremiumProtectionMode(written))
			_, errs := cfg.Validate()
			require.Empty(t, errsContaining(errs, "premiumProtection.mode"), "%q must be accepted", written)
			require.Equal(t, want, NormalizePremiumProtectionMode(PremiumProtectionMode(written)))
		}
	})

	t.Run("everything else is rejected", func(t *testing.T) {
		for _, written := range []string{"off", "OFF", "false", "true", "no", "0", "enabled", "bogus"} {
			cfg := cfgWithMode(PremiumProtectionMode(written))
			_, errs := cfg.Validate()
			require.Len(t, errsContaining(errs, "must be one of none,list,all"), 1,
				"%q must be a configuration error", written)
			require.Equal(t, PremiumProtectionMode(written),
				NormalizePremiumProtectionMode(PremiumProtectionMode(written)),
				"an invalid mode must stay unchanged for Validate to reject")
		}
	})
}

package config

import (
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/util/configutil"
)

// DefaultConfig provides default settings for Bedrock Edition support.
// Bedrock support enables cross-play between Java and Bedrock players via:
// - Geyser: Protocol translator (Bedrock ↔ Java Edition)
// - Floodgate: Authentication system for offline Bedrock players
var DefaultConfig = Config{
	GeyserListenAddr: "localhost:25567", // TCP address where Gate listens for Geyser connections (localhost recommended)
	UsernameFormat:   ".%s",             // Prefix Bedrock usernames with "." to avoid conflicts
	FloodgateKeyPath: "floodgate.pem",   // Shared encryption key for Floodgate authentication
	Managed:          nil,               // Will use DefaultManaged when any managed field is specified
}

// DefaultBedrockConfig provides default settings for the flattened BedrockConfig structure.
var DefaultBedrockConfig = BedrockConfig{
	Enabled:          false,
	GeyserListenAddr: DefaultConfig.GeyserListenAddr,
	UsernameFormat:   DefaultConfig.UsernameFormat,
	FloodgateKeyPath: DefaultConfig.FloodgateKeyPath,
	Managed:          BoolOrManagedGeyser{},
}

// DefaultManaged provides default settings for managed Geyser Standalone.
var DefaultManaged = ManagedGeyser{
	Enabled:    false,
	JarURL:     "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
	DataDir:    ".geyser",
	JavaPath:   "java",
	AutoUpdate: true, // Always download if missing or update available
}

// Config configures Bedrock Edition support via Geyser protocol translation and Floodgate authentication.
// This enables cross-play between Java Edition and Bedrock Edition (mobile, console, Windows) players.
//
// Bedrock support requires a Geyser instance to translate between protocols. You can either:
// 1. Use managed mode (recommended): Gate automatically handles Geyser for you
// 2. Run external Geyser: You manage your own Geyser instance
type Config struct {
	// Gate ↔ Geyser connection settings
	GeyserListenAddr string `yaml:"geyserListenAddr,omitempty" json:"geyserListenAddr,omitempty"` // TCP address where Gate listens for Geyser connections

	// Bedrock player settings
	UsernameFormat string `yaml:"usernameFormat,omitempty" json:"usernameFormat,omitempty"` // Format for Bedrock player usernames to avoid conflicts with Java players (e.g., ".%s")

	// Floodgate authentication (enables offline Bedrock players)
	FloodgateKeyPath string `yaml:"floodgateKeyPath,omitempty" json:"floodgateKeyPath,omitempty"` // Path to Floodgate AES encryption key shared with backend servers

	// Managed Geyser (recommended): Gate automatically handles Geyser process
	Managed *ManagedGeyser `yaml:"managed,omitempty" json:"managed,omitempty"` // Automatic Geyser JAR management and process control
}

// ManagedGeyser configures automatic Geyser Standalone management.
// When enabled, Gate automatically downloads, configures, starts, and updates Geyser.
// This is the recommended approach for most users.
//
// Note: The Bedrock port (default 19132) should be configured via ConfigOverrides:
//
//	configOverrides:
//	  bedrock:
//	    port: 19133  # Custom Bedrock port
type ManagedGeyser struct {
	Enabled         bool           `yaml:"enabled,omitempty" json:"enabled,omitempty"`                 // Enable managed Geyser mode (Gate handles Geyser process)
	JarURL          string         `yaml:"jarUrl,omitempty" json:"jarUrl,omitempty"`                   // Download URL for Geyser Standalone JAR
	DataDir         string         `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`                 // Directory for JAR and runtime data
	JavaPath        string         `yaml:"javaPath,omitempty" json:"javaPath,omitempty"`               // Path to Java executable
	AutoUpdate      bool           `yaml:"autoUpdate,omitempty" json:"autoUpdate,omitempty"`           // Download latest JAR on startup
	ExtraArgs       []string       `yaml:"extraArgs,omitempty" json:"extraArgs,omitempty"`             // Additional JVM arguments
	ConfigOverrides map[string]any `yaml:"configOverrides,omitempty" json:"configOverrides,omitempty"` // Custom overrides for auto-generated Geyser config
}

// GetManaged returns the managed config with defaults applied.
// If managed is not configured (nil), returns DefaultManaged.
// If managed is configured, merges user values with defaults.
func (c *Config) GetManaged() ManagedGeyser {
	if c.Managed == nil {
		return DefaultManaged
	}

	managed := DefaultManaged // Start with defaults

	// Override with user-specified values (only non-zero values override defaults)
	managed.Enabled = c.Managed.Enabled // Always take user's enabled value
	if c.Managed.JarURL != "" {
		managed.JarURL = c.Managed.JarURL
	}
	if c.Managed.DataDir != "" {
		managed.DataDir = c.Managed.DataDir
	}
	if c.Managed.JavaPath != "" {
		managed.JavaPath = c.Managed.JavaPath
	}
	// AutoUpdate: only override if user has non-zero ExtraArgs (indicating they set other fields)
	// This is a heuristic since we can't distinguish unset bool from explicit false
	if len(c.Managed.ExtraArgs) > 0 || c.Managed.JarURL != "" || c.Managed.DataDir != "" || c.Managed.JavaPath != "" {
		// User specified other fields, so they might have intentionally set AutoUpdate
		managed.AutoUpdate = c.Managed.AutoUpdate
	}
	// Otherwise keep default AutoUpdate = true

	if c.Managed.ExtraArgs != nil {
		managed.ExtraArgs = c.Managed.ExtraArgs
	}
	if c.Managed.ConfigOverrides != nil {
		managed.ConfigOverrides = c.Managed.ConfigOverrides
	}

	return managed
}

// BedrockConfig is the main Bedrock configuration that can be embedded in other configs.
// It supports a flattened structure (no nested "config" key) and flexible managed configuration.
type BedrockConfig struct {
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Flattened Bedrock configuration (removed nested config key)
	// Gate ↔ Geyser connection settings
	GeyserListenAddr string `yaml:"geyserListenAddr,omitempty" json:"geyserListenAddr,omitempty"`

	// Bedrock player settings
	UsernameFormat string `yaml:"usernameFormat,omitempty" json:"usernameFormat,omitempty"`

	// Floodgate authentication (enables offline Bedrock players)
	FloodgateKeyPath string `yaml:"floodgateKeyPath,omitempty" json:"floodgateKeyPath,omitempty"`

	// Managed Geyser (recommended): Gate automatically handles Geyser process
	// Can be either bool (true) or ManagedGeyser struct for advanced config
	Managed BoolOrManagedGeyser `yaml:"managed,omitempty" json:"managed,omitempty"`
}

// BoolOrManagedGeyser represents a field that can be either:
// - bool: true/false for simple enable/disable
// - ManagedGeyser: advanced managed configuration struct
type BoolOrManagedGeyser = configutil.BoolOrStruct[ManagedGeyser]

// Ensure BedrockConfig implements marshaling interfaces at compile time.
var (
	_ yaml.Unmarshaler = (*BedrockConfig)(nil)
)

// ToConfig converts the flattened BedrockConfig to the original Config structure
func (bc *BedrockConfig) ToConfig() Config {
	cfg := Config{
		GeyserListenAddr: bc.GeyserListenAddr,
		UsernameFormat:   bc.UsernameFormat,
		FloodgateKeyPath: bc.FloodgateKeyPath,
	}

	// Apply defaults if empty
	if cfg.GeyserListenAddr == "" {
		cfg.GeyserListenAddr = DefaultConfig.GeyserListenAddr
	}
	if cfg.UsernameFormat == "" {
		cfg.UsernameFormat = DefaultConfig.UsernameFormat
	}
	if cfg.FloodgateKeyPath == "" {
		cfg.FloodgateKeyPath = DefaultConfig.FloodgateKeyPath
	}

	// Handle managed config
	if !bc.Managed.IsNil() {
		managed := bc.GetManagedConfig()
		cfg.Managed = &managed
	}

	return cfg
}

// GetManagedConfig returns the managed configuration, handling both bool and struct forms
func (bc *BedrockConfig) GetManagedConfig() ManagedGeyser {
	if bc.Managed.IsNil() {
		return DefaultManaged
	}

	// Handle managed: true (shorthand)
	if bc.Managed.IsBool() {
		if bc.Managed.BoolValue() {
			managed := DefaultManaged
			managed.Enabled = true
			return managed
		}
		return DefaultManaged
	}

	// Handle managed as actual ManagedGeyser struct
	managedStruct := bc.Managed.StructValue()

	// Start with defaults and apply user values using the original logic
	managed := DefaultManaged
	managed.Enabled = managedStruct.Enabled // Always take user's enabled value

	if managedStruct.JarURL != "" {
		managed.JarURL = managedStruct.JarURL
	}
	if managedStruct.DataDir != "" {
		managed.DataDir = managedStruct.DataDir
	}
	if managedStruct.JavaPath != "" {
		managed.JavaPath = managedStruct.JavaPath
	}
	// AutoUpdate: only override if user has non-zero ExtraArgs (indicating they set other fields)
	// This is a heuristic since we can't distinguish unset bool from explicit false
	if len(managedStruct.ExtraArgs) > 0 || managedStruct.JarURL != "" || managedStruct.DataDir != "" || managedStruct.JavaPath != "" {
		// User specified other fields, so they might have intentionally set AutoUpdate
		managed.AutoUpdate = managedStruct.AutoUpdate
	}
	// Otherwise keep default AutoUpdate = true

	if managedStruct.ExtraArgs != nil {
		managed.ExtraArgs = managedStruct.ExtraArgs
	}
	if managedStruct.ConfigOverrides != nil {
		managed.ConfigOverrides = managedStruct.ConfigOverrides
	}

	return managed
}

// UnmarshalYAML implements custom YAML unmarshaling to handle managed: true shorthand
func (bc *BedrockConfig) UnmarshalYAML(node *yaml.Node) error {
	// First unmarshal into a temporary structure
	type tempBedrockConfig BedrockConfig
	temp := &tempBedrockConfig{}

	if err := node.Decode(temp); err != nil {
		return err
	}

	*bc = BedrockConfig(*temp)

	// Handle managed: true shorthand - if managed is true, also set enabled to true
	if bc.Managed.IsBool() && bc.Managed.BoolValue() {
		bc.Enabled = true
	}

	return nil
}

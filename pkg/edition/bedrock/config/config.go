package config

// DefaultConfig provides default settings for Bedrock Edition support.
// Bedrock support enables cross-play between Java and Bedrock players via:
// - Geyser: Protocol translator (Bedrock ↔ Java Edition)
// - Floodgate: Authentication system for offline Bedrock players
var DefaultConfig = Config{
	GeyserListenAddr: "0.0.0.0:25567", // TCP address where Gate listens for Geyser connections
	UsernameFormat:   ".%s",           // Prefix Bedrock usernames with "." to avoid conflicts
	FloodgateKeyPath: "floodgate.pem", // Shared encryption key for Floodgate authentication
	Managed:          nil,             // Will use DefaultManaged when any managed field is specified
}

// DefaultManaged provides default settings for managed Geyser Standalone.
var DefaultManaged = ManagedGeyser{
	Enabled:     false,
	JarURL:      "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
	DataDir:     ".geyser",
	JavaPath:    "java",
	BedrockPort: 19132,
	AutoUpdate:  true, // Always download if missing or update available
}

// Config configures Bedrock Edition support via Geyser protocol translation and Floodgate authentication.
// This enables cross-play between Java Edition and Bedrock Edition (mobile, console, Windows 10) players.
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
type ManagedGeyser struct {
	Enabled         bool           `yaml:"enabled,omitempty" json:"enabled,omitempty"`                 // Enable managed Geyser mode (Gate handles Geyser process)
	JarURL          string         `yaml:"jarUrl,omitempty" json:"jarUrl,omitempty"`                   // Download URL for Geyser Standalone JAR
	DataDir         string         `yaml:"dataDir,omitempty" json:"dataDir,omitempty"`                 // Directory for JAR and runtime data
	JavaPath        string         `yaml:"javaPath,omitempty" json:"javaPath,omitempty"`               // Path to Java executable
	BedrockPort     int            `yaml:"bedrockPort,omitempty" json:"bedrockPort,omitempty"`         // UDP port for Bedrock clients
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
	if c.Managed.BedrockPort != 0 {
		managed.BedrockPort = c.Managed.BedrockPort
	}
	// AutoUpdate: only override if user has non-zero ExtraArgs (indicating they set other fields)
	// This is a heuristic since we can't distinguish unset bool from explicit false
	if len(c.Managed.ExtraArgs) > 0 || c.Managed.JarURL != "" || c.Managed.DataDir != "" || c.Managed.JavaPath != "" || c.Managed.BedrockPort != 0 {
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

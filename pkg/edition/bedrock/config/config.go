package config

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	ReadTimeout:      30000, // 30 seconds
	GeyserListenAddr: "0.0.0.0:25567",
	UsernameFormat:   ".%s",
	FloodgateKeyPath: "floodgate.pem",
}

type Config struct {
	ReadTimeout      int    `yaml:"readTimeout,omitempty" json:"readTimeout,omitempty"`           // milliseconds
	GeyserListenAddr string `yaml:"geyserListenAddr,omitempty" json:"geyserListenAddr,omitempty"` // Address for Geyser to connect to
	UsernameFormat   string `yaml:"usernameFormat,omitempty" json:"usernameFormat,omitempty"`     // Format for Bedrock usernames (e.g., ".%s")
	FloodgateKeyPath string `yaml:"floodgateKeyPath,omitempty" json:"floodgateKeyPath,omitempty"` // Path to Floodgate encryption key
}

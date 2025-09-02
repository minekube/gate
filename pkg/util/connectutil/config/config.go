package config

const DefaultWatchServiceAddr = "wss://watch-connect.minekube.net"

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Enabled:            false,
	WatchServiceAddr:   DefaultWatchServiceAddr,
	Name:               "",
	EnforcePassthrough: false,
	TokenFilePath:      tokenFilename,
	Service: Service{
		Enabled:                 false,
		Addr:                    "localhost:8443",
		PublicTunnelServiceAddr: "ws://localhost:8080/tunnel",
		OverrideRegistration:    false,
	},
}

// Config is the config for Connect.
type Config struct {
	Enabled            bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`                       // Whether to connect Gate to the WatchService
	Name               string `yaml:"name,omitempty" json:"name,omitempty"`                             // Endpoint name
	EnforcePassthrough bool   `yaml:"enforcePassthrough,omitempty" json:"enforcePassthrough,omitempty"` // Setting to true will reject all sessions in non-passthrough mode.
	WatchServiceAddr   string `yaml:"watchServiceAddr,omitempty" json:"watchServiceAddr,omitempty"`     // The address of the WatchService
	TokenFilePath      string `yaml:"tokenFilePath,omitempty" json:"tokenFilePath,omitempty"`           // Path to the token file

	Service Service
}

// Service is a config for defining self-hosted
// Connect service for single-instance use.
type Service struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"` // The address all services listen on.
	// The address provided to endpoints in session proposals.
	// If not specified falls back to Services.Addr.
	PublicTunnelServiceAddr string `yaml:"publicTunnelServiceAddr"`
	// Overrides servers with the same name.
	OverrideRegistration bool `yaml:"overrideRegistration"`
}

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
	Enabled                 bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`                                 // Whether to connect Gate to the WatchService
	Name                    string `yaml:"name,omitempty" json:"name,omitempty"`                                       // Endpoint name
	AllowOfflineModePlayers bool   `yaml:"allowOfflineModePlayers,omitempty" json:"allowOfflineModePlayers,omitempty"` // Allow offline mode players to join.
	EnforcePassthrough      bool   `yaml:"enforcePassthrough,omitempty" json:"enforcePassthrough,omitempty"`           // Setting to true will reject all sessions in non-passthrough mode.
	WatchServiceAddr        string `yaml:"watchServiceAddr,omitempty" json:"watchServiceAddr,omitempty"`               // The address of the WatchService
	TokenFilePath           string `yaml:"tokenFilePath,omitempty" json:"tokenFilePath,omitempty"`                     // Path to the token file

	// BedrockPrincipal configures verification of signed Bedrock principal v2
	// envelopes on session proposals.
	BedrockPrincipal BedrockPrincipal `yaml:"bedrockPrincipal,omitempty" json:"bedrockPrincipal,omitempty"`

	Service Service
}

// BedrockPrincipal configures the Bedrock principal v2 verifier
// (Connect capability "bedrock-verified-principal-v2").
//
// When Mode is "require" and the verifier is operational, Gate advertises the
// capability on the Watch connection and applies exactly one verified game
// profile per Bedrock session proposal, produced by the Connect SDK verifier.
// When the verifier is not ready the capability is not advertised and
// proposals carrying an envelope are rejected instead of downgraded to
// unverified profile data.
type BedrockPrincipal struct {
	// Mode is "off" (default) or "require".
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// Issuer, TrustDomain and Audience are the exact trusted values every
	// envelope must be bound to.
	Issuer      string `yaml:"issuer,omitempty" json:"issuer,omitempty"`
	TrustDomain string `yaml:"trustDomain,omitempty" json:"trustDomain,omitempty"`
	Audience    string `yaml:"audience,omitempty" json:"audience,omitempty"`
	// Keys maps a key ID (kid) to an unpadded base64url-encoded raw Ed25519
	// public key that is eligible to sign principal envelopes.
	Keys map[string]string `yaml:"keys,omitempty" json:"keys,omitempty"`
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

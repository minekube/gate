package config

import (
	"fmt"

	"go.minekube.com/gate/pkg/util/validation"
)

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Bind:                          "0.0.0.0:25565",
	OnlineMode:                    true,
	OnlineModeKickExistingPlayers: false,
	Forwarding: Forwarding{
		Mode:           LegacyForwardingMode,
		VelocitySecret: "",
	},
	Status: Status{
		ShowMaxPlayers: 1000,
		Motd:           "§bA Gate Proxy\n§bVisit ➞ §fgithub.com/minekube/gate",
		// Contains Gate's icon
		Favicon:         "data:image/png;base64,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",
		LogPingRequests: false,
	},
	Query: Query{
		Enabled:     false,
		Port:        25577,
		ShowPlugins: false,
	},
	AnnounceForge:                        false,
	Servers:                              map[string]string{},
	Try:                                  []string{},
	ForcedHosts:                          map[string][]string{},
	FailoverOnUnexpectedServerDisconnect: true,
	ConnectionTimeout:                    5000,
	ReadTimeout:                          30000,
	Quota: Quota{
		Connections: QuotaSettings{
			Enabled:    true,
			OPS:        5,
			Burst:      10,
			MaxEntries: 1000,
		},
		Logins: QuotaSettings{
			Enabled:    true,
			OPS:        0.4,
			Burst:      3,
			MaxEntries: 1000,
		},
	},
	Compression: Compression{
		Threshold: 256,
		Level:     -1,
	},
	ProxyProtocol:                       false,
	ShouldPreventClientProxyConnections: false,
	BungeePluginChannelEnabled:          true,
	BuiltinCommands:                     true,
	RequireBuiltinCommandPermissions:    false,
	AnnounceProxyCommands:               true,
	Debug:                               false,
	ShutdownReason:                      "§cGate proxy is shutting down...\nPlease reconnect in a moment!",
	ForceKeyAuthentication:              true,
}

// Config is the configuration of the proxy.
type Config struct {
	Bind string // The address to listen for connections.

	OnlineMode                    bool
	OnlineModeKickExistingPlayers bool

	Forwarding Forwarding
	Status     Status
	Query      Query
	// Whether the proxy should present itself as a
	// Forge/FML-compatible server. By default, this is disabled.
	AnnounceForge bool

	Servers                              map[string]string // name:address
	Try                                  []string          // Try server names order
	ForcedHosts                          ForcedHosts
	FailoverOnUnexpectedServerDisconnect bool

	ConnectionTimeout int // Write timeout
	ReadTimeout       int

	Quota         Quota
	Compression   Compression
	ProxyProtocol bool // Enable HA-Proxy protocol mode

	ShouldPreventClientProxyConnections bool // Sends player IP to Mojang on login

	BungeePluginChannelEnabled       bool
	BuiltinCommands                  bool
	RequireBuiltinCommandPermissions bool // Whether builtin commands require player permissions
	AnnounceProxyCommands            bool
	ForceKeyAuthentication           bool // Added in 1.19

	Debug          bool
	ShutdownReason string
}

type (
	ForcedHosts map[string][]string // virtualhost:server names
	Status      struct {
		ShowMaxPlayers  int
		Motd            string
		Favicon         string
		LogPingRequests bool
	}
	Query struct {
		Enabled     bool
		Port        int
		ShowPlugins bool
	}
	Forwarding struct {
		Mode           ForwardingMode
		VelocitySecret string // Used with "velocity" mode
	}
	Compression struct {
		Threshold int
		Level     int
	}
	// Quota is the config for rate limiting.
	Quota struct {
		Connections QuotaSettings // Limits new connections per second, per IP block.
		Logins      QuotaSettings // Limits logins per second, per IP block.
		// Maybe add a bytes-per-sec limiter, or should be managed by a higher layer.
	}
	QuotaSettings struct {
		Enabled    bool    // If false, there is no such limiting.
		OPS        float32 // Allowed operations/events per second, per IP block
		Burst      int     // The maximum events per second, per block; the size of the token bucket
		MaxEntries int     // Maximum number of IP blocks to keep track of in cache
	}
)

// ForwardingMode is a player info forwarding mode.
type ForwardingMode string

const (
	NoneForwardingMode   ForwardingMode = "none"
	LegacyForwardingMode ForwardingMode = "legacy"
	// VelocityForwardingMode is a forwarding mode specified by the Velocity java proxy and
	// supported by PaperSpigot for versions starting at 1.13.
	VelocityForwardingMode ForwardingMode = "velocity"
)

// Validate validates Config.
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...any) { warns = append(warns, fmt.Errorf(m, args...)) }

	if c == nil {
		e("config must not be nil")
		return
	}

	if len(c.Bind) == 0 {
		e("Bind is empty")
	} else {
		if err := validation.ValidHostPort(c.Bind); err != nil {
			e("Invalid bind %q: %v", c.Bind, err)
		}
	}

	if !c.OnlineMode {
		w("Proxy is running in offline mode!")
	}

	switch c.Forwarding.Mode {
	case NoneForwardingMode:
		w("Player forwarding is disabled! Backend servers will have players with " +
			"offline-mode UUIDs and the same IP as the proxy.")
	case LegacyForwardingMode, VelocityForwardingMode:
	default:
		e("Unknown forwarding mode %q, must be one of none,legacy,velocity", c.Forwarding.Mode)
	}

	if len(c.Servers) == 0 {
		w("No backend servers configured.")
	}

	for name, addr := range c.Servers {
		if !validation.ValidServerName(name) {
			e("Invalid server name format %q: %s and length be 1-%d", name,
				validation.QualifiedNameErrMsg, validation.QualifiedNameMaxLength)
		}
		if err := validation.ValidHostPort(addr); err != nil {
			e("Invalid address %q for server %q: %w", addr, name, err)
		}
	}

	for _, name := range c.Try {
		if _, ok := c.Servers[name]; !ok {
			e("Fallback/try server %q must be registered under servers", name)
		}
	}

	for host, servers := range c.ForcedHosts {
		for _, name := range servers {
			e("Forced host %q server %q must be registered under servers", host, name)
		}
	}

	if c.Compression.Level < -1 || c.Compression.Level > 9 {
		e("Unsupported compression level %d: must be -1..9", c.Compression.Level)
	} else if c.Compression.Level == 0 {
		w("All packets going through the proxy will are uncompressed, this increases bandwidth usage.")
	}

	if c.Compression.Threshold < -1 {
		e("Invalid compression threshold %d: must be >= -1", c.Compression.Threshold)
	} else if c.Compression.Threshold == 0 {
		w("All packets going through the proxy will be compressed, this lowers bandwidth, " +
			"but has lower throughput and increases CPU usage.")
	}

	for _, quota := range []QuotaSettings{c.Quota.Connections, c.Quota.Logins} {
		if quota.Enabled {
			if quota.OPS <= 0 {
				e("Invalid quota ops %d, use a number > 0", quota.OPS)
			}
			if quota.Burst < 1 {
				e("Invalid quota burst %d, use a number >= 1", quota.Burst)
			}
			if quota.MaxEntries < 1 {
				e("Invalid quota max entries %d, use a number >= 1", quota.Burst)
			}
		}
	}

	return
}

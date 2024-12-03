package config

import (
	"fmt"
	"strings"
	"time"

	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/validation"
)

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Bind:                          "0.0.0.0:25565",
	OnlineMode:                    true,
	Auth:                          Auth{},
	OnlineModeKickExistingPlayers: false,
	Forwarding: Forwarding{
		Mode:           LegacyForwardingMode,
		VelocitySecret: "",
	},
	Status: Status{
		ShowMaxPlayers: 1000,
		Motd:           defaultMotd(),
		// Contains Gate's icon
		Favicon:         "",
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
	ConnectionTimeout:                    configutil.Duration(5000 * time.Millisecond),
	ReadTimeout:                          configutil.Duration(30000 * time.Millisecond),
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
	ProxyProtocolBackend:                false,
	ShouldPreventClientProxyConnections: false,
	BungeePluginChannelEnabled:          true,
	BuiltinCommands:                     true,
	RequireBuiltinCommandPermissions:    false,
	AnnounceProxyCommands:               true,
	Debug:                               false,
	ShutdownReason:                      defaultShutdownReason(),
	ForceKeyAuthentication:              true,
	Lite:                                liteconfig.DefaultConfig,
}

func defaultMotd() *configutil.TextComponent {
	return text("§bA Gate Proxy\n§bVisit ➞ §fgithub.com/minekube/gate")
}

func defaultShutdownReason() *configutil.TextComponent {
	return text("§cGate proxy is shutting down...\nPlease reconnect in a moment!")
}

// Config is the configuration of the proxy.
type Config struct {
	Bind string `yaml:"bind"` // The address to listen for connections.

	OnlineMode                    bool `yaml:"onlineMode,omitempty" json:"onlineMode,omitempty"`                                       // Whether to enable online mode.
	Auth                          Auth `yaml:"auth,omitempty" json:"auth,omitempty"`                                                   // Authentication settings.
	OnlineModeKickExistingPlayers bool `yaml:"onlineModeKickExistingPlayers,omitempty" json:"onlineModeKickExistingPlayers,omitempty"` // Kicks existing players when a premium player with the same name joins.

	Forwarding Forwarding `yaml:"forwarding,omitempty" json:"forwarding,omitempty"` // Player info forwarding settings.
	Status     Status     `yaml:"status,omitempty" json:"status,omitempty"`         // Status response settings.
	Query      Query      `yaml:"query,omitempty" json:"query,omitempty"`           // Query settings.
	// Whether the proxy should present itself as a
	// Forge/FML-compatible server. By default, this is disabled.
	AnnounceForge bool `yaml:"announceForge,omitempty" json:"announceForge,omitempty"`

	Servers                              map[string]string `yaml:"servers,omitempty" json:"servers,omitempty"` // name:address
	Try                                  []string          `yaml:"try,omitempty" json:"try,omitempty"`         // Try server names order
	ForcedHosts                          ForcedHosts       `yaml:"forcedHosts,omitempty" json:"forcedHosts,omitempty"`
	FailoverOnUnexpectedServerDisconnect bool              `yaml:"failoverOnUnexpectedServerDisconnect,omitempty" json:"failoverOnUnexpectedServerDisconnect,omitempty"`

	ConnectionTimeout configutil.Duration `yaml:"connectionTimeout,omitempty" json:"connectionTimeout,omitempty"` // Write timeout
	ReadTimeout       configutil.Duration `yaml:"readTimeout,omitempty" json:"readTimeout,omitempty"`             // Read timeout

	Quota                Quota       `yaml:"quota,omitempty" json:"quota,omitempty"` // Rate limiting settings
	Compression          Compression `yaml:"compression,omitempty" json:"compression,omitempty"`
	ProxyProtocol        bool        `yaml:"proxyProtocol,omitempty" json:"proxyProtocol,omitempty"`     // Enable HA-Proxy protocol mode
	ProxyProtocolBackend bool        `yaml:"proxyProtocolBackend" json:"proxyProtocolBackend,omitempty"` // Enable HA-Proxy protocol mode for backend servers

	ShouldPreventClientProxyConnections bool `yaml:"shouldPreventClientProxyConnections" json:"shouldPreventClientProxyConnections,omitempty"` // Sends player IP to Mojang on login

	AcceptTransfers                  bool `yaml:"acceptTransfers,omitempty" json:"acceptTransfers,omitempty"`                                   // Whether to accept transfers from other hosts via transfer packet
	BungeePluginChannelEnabled       bool `yaml:"bungeePluginChannelEnabled,omitempty" json:"bungeePluginChannelEnabled,omitempty"`             // Whether to enable BungeeCord plugin messaging
	BuiltinCommands                  bool `yaml:"builtinCommands,omitempty" json:"builtinCommands,omitempty"`                                   // Whether to enable builtin commands
	RequireBuiltinCommandPermissions bool `yaml:"requireBuiltinCommandPermissions,omitempty" json:"requireBuiltinCommandPermissions,omitempty"` // Whether builtin commands require player permissions
	AnnounceProxyCommands            bool `yaml:"announceProxyCommands,omitempty" json:"announceProxyCommands,omitempty"`                       // Whether to announce proxy commands to players
	ForceKeyAuthentication           bool `yaml:"forceKeyAuthentication,omitempty" json:"forceKeyAuthentication,omitempty"`                     // Added in 1.19

	Debug          bool                      `yaml:"debug,omitempty" json:"debug,omitempty"` // Enable debug mode
	ShutdownReason *configutil.TextComponent `yaml:"shutdownReason,omitempty" json:"shutdownReason,omitempty"`

	Lite liteconfig.Config `yaml:"lite,omitempty" json:"lite,omitempty"` // Lite mode settings
}

type (
	ForcedHosts map[string][]string // virtualhost:server names
	Status      struct {
		ShowMaxPlayers  int                       `yaml:"showMaxPlayers"`
		Motd            *configutil.TextComponent `yaml:"motd"`
		Favicon         favicon.Favicon           `yaml:"favicon"`
		LogPingRequests bool                      `yaml:"logPingRequests"`
	}
	Query struct {
		Enabled     bool `yaml:"enabled"`
		Port        int  `yaml:"port"`
		ShowPlugins bool `yaml:"showPlugins"`
	}
	Forwarding struct {
		Mode              ForwardingMode `yaml:"mode"`
		VelocitySecret    string         `yaml:"velocitySecret"`    // Used with "velocity" mode
		BungeeGuardSecret string         `yaml:"bungeeGuardSecret"` // Used with "bungeeguard" mode
	}
	Compression struct {
		Threshold int `yaml:"threshold"`
		Level     int `yaml:"level"`
	}
	// Quota is the config for rate limiting.
	Quota struct {
		Connections QuotaSettings `yaml:"connections"` // Limits new connections per second, per IP block.
		Logins      QuotaSettings `yaml:"logins"`      // Limits logins per second, per IP block.
		// Maybe add a bytes-per-sec limiter, or should be managed by a higher layer.
	}
	QuotaSettings struct {
		Enabled    bool    `yaml:"enabled"`    // If false, there is no such limiting.
		OPS        float32 `yaml:"ops"`        // Allowed operations/events per second, per IP block
		Burst      int     `yaml:"burst"`      // The maximum events per second, per block; the size of the token bucket
		MaxEntries int     `yaml:"maxEntries"` // Maximum number of IP blocks to keep track of in cache
	}
	// Auth is the config for authentication.
	Auth struct {
		// SessionServerURL is the base URL for the Mojang session server to authenticate online mode players.
		// Defaults to https://sessionserver.mojang.com/session/minecraft/hasJoined
		SessionServerURL *configutil.URL `yaml:"sessionServerUrl"` // TODO support multiple urls configutil.SingleOrMulti[URL]
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
	// BungeeGuardForwardingMode is a forwarding mode used by versions lower than 1.13
	BungeeGuardForwardingMode ForwardingMode = "bungeeguard"
)

// Validate validates Config.
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...any) { warns = append(warns, fmt.Errorf(m, args...)) }

	if c == nil {
		e("config must not be nil")
		return
	}

	if strings.TrimSpace(c.Bind) == "" {
		e("Bind is empty")
	} else {
		if err := validation.ValidHostPort(c.Bind); err != nil {
			e("Invalid bind %q: %v", c.Bind, err)
		}
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

	if c.Lite.Enabled {
		return c.Lite.Validate()
	}

	if !c.OnlineMode {
		w("Proxy is running in offline mode!")
	}

	switch c.Forwarding.Mode {
	case NoneForwardingMode:
		w("Player forwarding is disabled! Backend servers will have players with " +
			"offline-mode UUIDs and the same IP as the proxy.")
	case LegacyForwardingMode, VelocityForwardingMode, BungeeGuardForwardingMode:
	default:
		e("Unknown forwarding mode %q, must be one of none,legacy,velocity,bungeeguard", c.Forwarding.Mode)
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

	return
}

func text(s string) *configutil.TextComponent {
	return (*configutil.TextComponent)(must(componentutil.ParseTextComponent(
		version.MinimumVersion.Protocol, s)))
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

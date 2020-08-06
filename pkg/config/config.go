package config

import (
	"errors"
	"fmt"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"net"
	"regexp"
)

// Config is the configuration of the proxy.
type Config struct {
	*File
}

// File is for reading a config file into this struct.
type File struct {
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

	Quota                               Quota
	Compression                         Compression
	ProxyProtocol                       bool // ha-proxy compatibility
	ShouldPreventClientProxyConnections bool // sends player ip to mojang

	BungeePluginChannelEnabled bool

	Debug            bool
	ConfigAutoUpdate bool
}

type (
	ForcedHosts map[string][]string // virtualhost:server names
	Status      struct {
		MaxPlayers       int
		Motd             string
		FavIconFile      string
		ShowPingRequests bool
	}
	Query struct {
		Enabled     bool
		Port        int
		ShowPlugins bool
	}
	Forwarding struct {
		Mode           ForwardingMode
		VelocitySecret string
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

func init() {
	viper.SetDefault("bind", "0.0.0.0:25565")
	viper.SetDefault("onlineMode", true)
	viper.SetDefault("forwarding.mode", LegacyForwardingMode)
	viper.SetDefault("announceForge", false)

	viper.SetDefault("status.motd", "§bA Gate Proxy Server!")
	viper.SetDefault("status.maxplayers", 1000)
	viper.SetDefault("status.faviconfile", "server-icon.png")
	viper.SetDefault("status.showPingRequests", false)

	viper.SetDefault("compression.threshold", 256)
	viper.SetDefault("compression.level", 1)

	viper.SetDefault("query.enabled", false)
	viper.SetDefault("query.port", 25577)
	viper.SetDefault("query.showplugins", false)

	// Default quotas should never affect legitimate operations,
	// but rate limits aggressive behaviours.
	viper.SetDefault("quota.connections.Enabled", true)
	viper.SetDefault("quota.connections.OPS", 5)
	viper.SetDefault("quota.connections.burst", 10)
	viper.SetDefault("quota.connections.MaxEntries", 1000)

	viper.SetDefault("quota.logins.Enabled", true)
	viper.SetDefault("quota.logins.OPS", 0.4)
	viper.SetDefault("quota.logins.burst", 3)
	viper.SetDefault("quota.logins.MaxEntries", 1000)

	viper.SetDefault("connectiontimeout", 5000)
	viper.SetDefault("readtimeout", 30000)
	viper.SetDefault("BungeePluginChannelEnabled", true)
	viper.SetDefault("FailoverOnUnexpectedServerDisconnect", true)
	viper.SetDefault("configautoupdate", false)
}

func NewValid(f *File) (c *Config, err error) {
	if f == nil {
		return nil, errors.New("file must not be nil-pointer")
	}

	// validate input config
	warns, errs := validate(f)
	if len(errs) != 0 {
		for _, err = range errs {
			zap.S().Errorf("Config error: %s", err)
		}

		a, s := "are", "s"
		if len(errs) == 1 {
			a, s = "is", ""
		}
		return nil, fmt.Errorf("there %s %d config validation error%s", a, len(errs), s)
	}
	for _, err = range warns {
		zap.L().Warn(err.Error())
	}

	return &Config{
		File: f,
	}, nil
}

func validate(f *File) (warns []error, errs []error) {
	e := func(m string, args ...interface{}) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...interface{}) { warns = append(warns, fmt.Errorf(m, args...)) }

	if len(f.Bind) == 0 {
		e("bind is empty")
	} else {
		if err := ValidHostPort(f.Bind); err != nil {
			e("invalid bind %q: %v", f.Bind, err)
		}
	}

	if !f.OnlineMode {
		w("Proxy is running in offline mode!")
	}

	switch f.Forwarding.Mode {
	case NoneForwardingMode:
		w("Player forwarding is disabled! Backend servers will have players with " +
			"offline-mode UUIDs and the same IP as the proxy.")
	case LegacyForwardingMode, VelocityForwardingMode:
	default:
		e("Unknown forwarding mode %q, must be one of none,legacy,velocity", f.Forwarding.Mode)
	}

	if len(f.Servers) == 0 {
		w("No backend servers configured.")
	}
	for name, addr := range f.Servers {
		if !ValidServerName(name) {
			e("Invalid server name format %q: %s and length be 1-%d", name,
				qualifiedNameErrMsg, qualifiedNameMaxLength)
		}
		if err := ValidHostPort(addr); err != nil {
			e("Invalid address %q for server %q: %w", addr, name, err)
		}
	}

	for _, name := range f.Try {
		if _, ok := f.Servers[name]; !ok {
			e("Fallback/try server %q must be registered under servers", name)
		}
	}

	for host, servers := range f.ForcedHosts {
		for _, name := range servers {
			e("Forced host %q server %q must be registered under servers", host, name)
		}
	}

	if f.Compression.Level < -1 || f.Compression.Level > 9 {
		e("Unsupported compression level %d: must be -1..9", f.Compression.Level)
	} else if f.Compression.Level == 0 {
		w("All packets going through the proxy will are uncompressed, this increases bandwidth usage.")
	}

	if f.Compression.Threshold < -1 {
		e("Invalid compression threshold %d: must be >= -1", f.Compression.Threshold)
	} else if f.Compression.Threshold == 0 {
		w("All packets going through the proxy will be compressed, this lowers bandwidth, " +
			"but has lower throughput and increases CPU usage.")
	}

	for _, quota := range []QuotaSettings{f.Quota.Connections, f.Quota.Logins} {
		if quota.Enabled {
			if quota.OPS < 1 {
				e("Invalid quota ops %d, use a number >= 1", quota.OPS)
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

func ValidHostPort(hostAndPort string) error {
	_, _, err := net.SplitHostPort(hostAndPort)
	return err
}

// Constants obtained from https://github.com/kubernetes/apimachinery/blob/master/pkg/util/validation/validation.go
const (
	qnameCharFmt           = "[A-Za-z0-9]"
	qnameExtCharFmt        = "[-A-Za-z0-9_.]"
	qualifiedNameFmt       = "(" + qnameCharFmt + qnameExtCharFmt + "*)?" + qnameCharFmt
	qualifiedNameErrMsg    = "must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character"
	qualifiedNameMaxLength = 63
)

var qualifiedNameRegexp = regexp.MustCompile("^" + qualifiedNameFmt + "$")

func ValidServerName(str string) bool {
	return str != "" && len(str) <= qualifiedNameMaxLength && qualifiedNameRegexp.MatchString(str)
}

// ForwardingMode is a player info forwarding mode.
type ForwardingMode string

const (
	NoneForwardingMode   ForwardingMode = "none"
	LegacyForwardingMode ForwardingMode = "legacy"
	// A forwarding mode specified by the Velocity java proxy and
	// supported by PaperSpigot for versions starting at 1.13.
	VelocityForwardingMode ForwardingMode = "velocity"
)

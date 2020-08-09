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

	Debug  bool
	Health HealthProbeService
}

type (
	ForcedHosts map[string][]string // virtualhost:server names
	Status      struct {
		ShowMaxPlayers   int
		Motd             string
		Favicon          string
		ShowPingRequests bool
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
	// GRPC health probe service to use with Kubernetes pods.
	// (https://github.com/grpc-ecosystem/grpc-health-probe)
	HealthProbeService struct {
		Enabled bool
		Bind    string
	}
)

// ForwardingMode is a player info forwarding mode.
type ForwardingMode string

const (
	NoneForwardingMode   ForwardingMode = "none"
	LegacyForwardingMode ForwardingMode = "legacy"
	// A forwarding mode specified by the Velocity java proxy and
	// supported by PaperSpigot for versions starting at 1.13.
	VelocityForwardingMode ForwardingMode = "velocity"
)

// Init config defaults
func init() {
	viper.SetDefault("bind", "0.0.0.0:25565")
	viper.SetDefault("onlineMode", true)
	viper.SetDefault("forwarding.mode", LegacyForwardingMode)

	viper.SetDefault("status.motd", "§bA Gate Proxy §7(Alpha)\n§bVisit ➞ §fgithub.com/minekube/gate")
	viper.SetDefault("status.showmaxplayers", 1000)
	viper.SetDefault("status.announceForge", false)
	viper.SetDefault("status.showPingRequests", false)

	viper.SetDefault("compression.threshold", 256)
	viper.SetDefault("compression.level", -1)

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

	viper.SetDefault("Health.enabled", false)
	viper.SetDefault("Health.bind", "0.0.0.0:8080")
}

func Validate(c *Config) (err error) {
	if c == nil {
		return errors.New("config must not be nil-pointer")
	}

	// validate input config
	warns, errs := validate(c)
	if len(errs) != 0 {
		for _, err = range errs {
			zap.S().Errorf("Config error: %s", err)
		}

		a, s := "are", "s"
		if len(errs) == 1 {
			a, s = "is", ""
		}
		return fmt.Errorf("there %s %d config validation error%s", a, len(errs), s)
	}
	for _, err = range warns {
		zap.L().Warn(err.Error())
	}
	return nil
}

func validate(c *Config) (warns []error, errs []error) {
	e := func(m string, args ...interface{}) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...interface{}) { warns = append(warns, fmt.Errorf(m, args...)) }

	if len(c.Bind) == 0 {
		e("Bind is empty")
	} else {
		if err := ValidHostPort(c.Bind); err != nil {
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
		if !ValidServerName(name) {
			e("Invalid server name format %q: %s and length be 1-%d", name,
				qualifiedNameErrMsg, qualifiedNameMaxLength)
		}
		if err := ValidHostPort(addr); err != nil {
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

	if c.Health.Enabled {
		if err := ValidHostPort(c.Health.Bind); err != nil {
			e("Invalid health probe bind address %q: %v", c.Health.Bind, err)
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

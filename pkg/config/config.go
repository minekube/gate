package config

import (
	"errors"
	"github.com/spf13/viper"
)

// Config is the configuration of the proxy.
type Config struct {
	File
}

// File is for reading a config file into this struct.
type File struct {
	// The address to listen for connections.
	Bind                          string
	OnlineMode                    bool
	OnlineModeKickExistingPlayers bool
	Forwarding                    struct {
		Mode           ForwardingMode
		VelocitySecret string
	}
	Status struct {
		MaxPlayers       int
		Motd             string
		FavIconFile      string
		ShowPingRequests bool
	}
	// Whether the proxy should present itself as a
	// Forge/FML-compatible server. By default, this is disabled.
	AnnounceForge bool
	Servers       map[string]string   // name:address
	Try           []string            // Try server names order
	ForcedHosts   map[string][]string // virtualhost:server names
	Query         struct {
		Enabled     bool
		Port        int
		ShowPlugins bool
	}
	ConnectionTimeout                    int
	ReadTimeout                          int
	FailoverOnUnexpectedServerDisconnect bool
	Compression                          struct {
		Threshold int
		Level     int
	}
	ShouldPreventClientProxyConnections bool // sends player ip to mojang
	BungeePluginChannelEnabled          bool
	ProxyProtocol                       bool // ha-proxy compatibility
	ConfigAutoUpdate                    bool
	Debug                               bool
}

func init() {
	viper.SetDefault("bind", "0.0.0.0:25565")
	viper.SetDefault("onlineMode", true)
	viper.SetDefault("forwarding.mode", LegacyForwardingMode)
	viper.SetDefault("announceForge", false)
	viper.SetDefault("status.motd", "§bA Gate Proxy Server!")
	viper.SetDefault("status.maxplayers", 1000)
	viper.SetDefault("status.faviconfile", "server-icon.png")
	viper.SetDefault("status.showPingRequests", false)
	//viper.SetDefault("compression.threshold", -1)
	viper.SetDefault("compression.threshold", 256) // TODO de/compression doesn't work yet
	viper.SetDefault("compression.level", 1)
	viper.SetDefault("query.enabled", false)
	viper.SetDefault("query.port", 25577)
	viper.SetDefault("query.showplugins", false)
	viper.SetDefault("loginratelimit", 3000)
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

	// TODO validate input config

	return &Config{
		File: *f,
	}, nil
}

func (c *Config) AttemptConnectionOrder() []string {
	return c.Try
}

type ForwardingMode string

const (
	NoneForwardingMode   ForwardingMode = "none"
	LegacyForwardingMode ForwardingMode = "legacy"
	// A forwarding mode specified by the Velocity java proxy and
	// supported by PaperSpigot for versions starting at 1.13.
	VelocityForwardingMode ForwardingMode = "velocity"
)

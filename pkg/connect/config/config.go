package config

import (
	"math/rand"
	"time"

	"github.com/rs/xid"
)

const DefaultWatchServiceAddr = "connect.api.minekube.com"

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Enabled:            false,
	WatchServiceAddr:   DefaultWatchServiceAddr,
	Name:               xid.New().String(),
	EnforcePassthrough: false,
	Insecure:           false,
	Services: Services{
		Watch: Watch{
			Enabled:                 false,
			PublicTunnelServiceAddr: "your-routable-address",
		},
		Tunnel: Tunnel{
			Enabled: false,
		},
	},
}

// Config is the config for Connect.
type Config struct {
	Enabled            bool // Whether to connect Gate to the WatchService
	Name               string
	EnforcePassthrough bool // Setting to true will reject all sessions in non-passthrough mode.
	WatchServiceAddr   string
	Insecure           bool // Whether to use transport security for dialing Connect services

	Services Services
}

// Services is a config for defining self-hosted Connect services.
type Services struct {
	Addr   string // The address all services listen on.
	Watch  Watch
	Tunnel Tunnel
}

type (
	Watch struct {
		Enabled bool
		// The address provided to watching clients in session proposals.
		// If not specified falls back to Services.Addr.
		PublicTunnelServiceAddr string
	}
	Tunnel struct {
		Enabled bool
	}
)

func init() { rand.Seed(time.Now().UnixNano()) }

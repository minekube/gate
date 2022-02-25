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
	Service: Service{
		Enabled:                 false,
		Addr:                    "localhost:8443",
		PublicTunnelServiceAddr: "localhost:8443",
		OverrideRegistration:    false,
	},
}

// Config is the config for Connect.
type Config struct {
	Enabled            bool // Whether to connect Gate to the WatchService
	Name               string
	EnforcePassthrough bool // Setting to true will reject all sessions in non-passthrough mode.
	WatchServiceAddr   string
	Insecure           bool // Whether to use transport security for dialing Connect services

	Service Service
}

// Service is a config for defining self-hosted
// Connect service for single-instance use.
type Service struct {
	Enabled bool
	Addr    string // The address all services listen on.
	// The address provided to endpoints in session proposals.
	// If not specified falls back to Services.Addr.
	PublicTunnelServiceAddr string
	// Overrides servers with the same name.
	OverrideRegistration bool
}

func init() { rand.Seed(time.Now().UnixNano()) }

package config

import (
	"fmt"
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/netutil"
)

// DefaultConfig is the default configuration for Lite mode.
var DefaultConfig = Config{
	Enabled: false,
	Routes:  []Route{},
}

type (
	// Config is the configuration for Lite mode.
	Config struct {
		Enabled bool
		Routes  []Route
	}
	Route struct {
		Host          configutil.SingleOrMulti[string] `json:"host" yaml:"host"`
		Backend       configutil.SingleOrMulti[string] `json:"backend" yaml:"backend"`
		CachePingTTL  time.Duration                    `json:"cachePingTTL,omitempty" yaml:"cachePingTTL,omitempty"` // 0 = default, < 0 = disabled
		Fallback      *Status                          `json:"fallback,omitempty" yaml:"fallback,omitempty"`         // nil = disabled
		ProxyProtocol bool                             `json:"proxyProtocol,omitempty" yaml:"proxyProtocol,omitempty"`
		RealIP        bool                             `json:"realIP,omitempty" yaml:"realIP,omitempty"`
	}
	Status struct {
		MOTD    string          `yaml:"motd,omitempty" json:"motd,omitempty"`
		Version ping.Version    `yaml:"version,omitempty" json:"version,omitempty"`
		Favicon favicon.Favicon `yaml:"favicon,omitempty" json:"favicon,omitempty"`
		ModInfo modinfo.ModInfo `yaml:"modInfo,omitempty" json:"modInfo,omitempty"`

		ParsedMOTD struct {
			Text      *component.Text `yaml:"-" json:"-"`
			sync.Once `yaml:"-" json:"-"`
		} `yaml:"-" json:"-"`
	}
)

// Response returns the configured status response.
func (s *Status) Response(protocol proto.Protocol) (*ping.ServerPing, error) {
	// Lazy parse MOTD
	var err error
	s.ParsedMOTD.Do(func() {
		s.ParsedMOTD.Text, err = componentutil.ParseTextComponent(protocol, s.MOTD)
	})
	if err != nil {
		return nil, err
	}

	return &ping.ServerPing{
		Version:     s.Version,
		Description: s.ParsedMOTD.Text,
		Favicon:     s.Favicon,
		ModInfo:     &s.ModInfo,
	}, nil
}

// GetCachePingTTL returns the configured ping cache TTL or a default duration if not set.
func (r *Route) GetCachePingTTL() time.Duration {
	const defaultTTL = time.Second * 10
	if r.CachePingTTL == 0 {
		return defaultTTL
	}
	return r.CachePingTTL
}

// CachePingEnabled returns true if the route has a ping cache enabled.
func (r *Route) CachePingEnabled() bool { return r.GetCachePingTTL() > 0 }

func (c Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }

	if len(c.Routes) == 0 {
		e("No routes configured")
		return
	}

	for i, ep := range c.Routes {
		if len(ep.Host) == 0 {
			e("Route %d: no host configured", i)
		}
		if len(ep.Backend) == 0 {
			e("Route %d: no backend configured", i)
		}
		for i, addr := range ep.Backend {
			_, err := netutil.Parse(addr, "tcp")
			if err != nil {
				e("Route %d: backend %d: failed to parse address: %w", i, err)
			}
		}
	}

	return
}

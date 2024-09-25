package config

import (
	"encoding/json"
	"fmt"
	"time"

	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/ping"
	"go.minekube.com/gate/pkg/gate/proto"
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
		Enabled bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
		Routes  []Route `yaml:"routes,omitempty" json:"routes,omitempty"`
	}
	Route struct {
		Host          configutil.SingleOrMulti[string] `json:"host,omitempty" yaml:"host,omitempty"`
		Backend       configutil.SingleOrMulti[string] `json:"backend,omitempty" yaml:"backend,omitempty"`
		CachePingTTL  configutil.Duration              `json:"cachePingTTL,omitempty" yaml:"cachePingTTL,omitempty"` // 0 = default, < 0 = disabled
		Fallback      *Status                          `json:"fallback,omitempty" yaml:"fallback,omitempty"`         // nil = disabled
		ProxyProtocol bool                             `json:"proxyProtocol,omitempty" yaml:"proxyProtocol,omitempty"`
		// Deprecated: use TCPShieldRealIP instead.
		RealIP            bool `json:"realIP,omitempty" yaml:"realIP,omitempty"`
		TCPShieldRealIP   bool `json:"tcpShieldRealIP,omitempty" yaml:"tcpShieldRealIP,omitempty"`
		ModifyVirtualHost bool `json:"modifyVirtualHost,omitempty" yaml:"modifyVirtualHost,omitempty"`
	}
	Status struct {
		MOTD    *configutil.TextComponent `yaml:"motd,omitempty" json:"motd,omitempty"`
		Version ping.Version              `yaml:"version,omitempty" json:"version,omitempty"`
		Favicon favicon.Favicon           `yaml:"favicon,omitempty" json:"favicon,omitempty"`
		ModInfo modinfo.ModInfo           `yaml:"modInfo,omitempty" json:"modInfo,omitempty"`
	}
)

// Response returns the configured status response.
func (s *Status) Response(proto.Protocol) (*ping.ServerPing, error) {
	return &ping.ServerPing{
		Version:     s.Version,
		Description: s.MOTD.T(),
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
	return time.Duration(r.CachePingTTL)
}

// CachePingEnabled returns true if the route has a ping cache enabled.
func (r *Route) CachePingEnabled() bool { return r.GetCachePingTTL() > 0 }

// GetTCPShieldRealIP returns the configured TCPShieldRealIP or deprecated RealIP value.
func (r *Route) GetTCPShieldRealIP() bool { return r.TCPShieldRealIP || r.RealIP }

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

// Equal returns true if the Routes are equal.
func (r *Route) Equal(other *Route) bool {
	j, err := json.Marshal(r)
	if err != nil {
		return false
	}
	o, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return string(j) == string(o)
}

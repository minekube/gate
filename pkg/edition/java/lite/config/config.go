package config

import (
	"fmt"
	"time"

	"go.minekube.com/gate/pkg/util/configutil"
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
		Host          configutil.SingleOrMulti[string]
		Backend       configutil.SingleOrMulti[string]
		CachePingTTL  time.Duration // 0 = default, < 0 = disabled
		ProxyProtocol bool
		RealIP        bool
	}
)

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
	}

	return
}

func (r *Route) GetCachePingTTL() time.Duration {
	const defaultTTL = time.Second * 10
	if r.CachePingTTL == 0 {
		return defaultTTL
	}
	return r.CachePingTTL
}

func (r *Route) CachePingEnabled() bool { return r.GetCachePingTTL() > 0 }

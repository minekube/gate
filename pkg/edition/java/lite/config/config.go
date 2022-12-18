package config

import (
	"fmt"

	"go.minekube.com/gate/pkg/util/configutil"
)

// DefaultConfig is the default configuration for Lite mode.
var DefaultConfig = Config{
	Enabled:   false,
	Endpoints: []Endpoint{},
}

type (
	// Config is the configuration for Lite mode.
	Config struct {
		Enabled   bool
		Endpoints []Endpoint
	}
	Endpoint struct {
		Host          configutil.SingleOrMulti[string]
		Backend       configutil.SingleOrMulti[string]
		ProxyProtocol bool
	}
)

func (c Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }

	if len(c.Endpoints) == 0 {
		e("no endpoints configured")
		return
	}

	for i, ep := range c.Endpoints {
		if len(ep.Host) == 0 {
			e("endpoint %d: no host configured", i)
		}
		if len(ep.Backend) == 0 {
			e("endpoint %d: no backend configured", i)
		}
	}

	return
}

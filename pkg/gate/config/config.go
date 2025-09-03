package config

import (
	"fmt"

	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/internal/api"
	connect "go.minekube.com/gate/pkg/util/connectutil/config"
	"go.minekube.com/gate/pkg/util/validation"
)

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Config: jconfig.DefaultConfig,
	HealthService: HealthService{
		Enabled: false,
		Bind:    "0.0.0.0:9090",
	},
	Connect: connect.DefaultConfig,
	API: API{
		Enabled: false,
		Config:  api.DefaultConfig,
	},
}

// Config is the root configuration of Gate.
type Config struct {
	// Config is the Java edition configuration (with embedded Bedrock config).
	Config jconfig.Config `yaml:"config,omitempty"`
	// See HealthService struct.
	HealthService HealthService `json:"healthService,omitempty" yaml:"healthService,omitempty"`
	// See Connect struct.
	Connect connect.Config `json:"connect,omitempty" yaml:"connect,omitempty"`
	// See API struct.
	API API `json:"api,omitempty" yaml:"api,omitempty"`
}

// HealthService is a GRPC health probe service for use with Kubernetes pods.
// (https://github.com/grpc-ecosystem/grpc-health-probe)
type HealthService struct {
	Enabled bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Bind    string `json:"bind,omitempty" yaml:"bind,omitempty"`
}

// API is the configuration for the Gate API.
type API struct {
	Enabled bool       `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Config  api.Config `json:"config,omitempty" yaml:"config,omitempty"`
}

// Validate validates a Config and all enabled configs.
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	if c == nil {
		e("config must not be nil")
		return
	}

	if c.HealthService.Enabled {
		if err := validation.ValidHostPort(c.HealthService.Bind); err != nil {
			e("Invalid health probe bind address %q: %v", c.HealthService.Bind, err)
		}
	}

	prefix := func(p string, errs []error) (pErrs []error) {
		for _, err := range errs {
			pErrs = append(pErrs, fmt.Errorf("%s: %w", p, err))
		}
		return
	}

	// Validate Java config (always enabled)
	warns2, errs2 := c.Config.Validate()
	warns = append(warns, prefix("java", warns2)...)
	errs = append(errs, prefix("java", errs2)...)

	// Validate Bedrock config if enabled
	if c.Config.Bedrock.Enabled {
		bedrockConfig := c.Config.Bedrock.ToConfig()
		warns2, errs2 := bedrockConfig.Validate()
		warns = append(warns, prefix("bedrock", warns2)...)
		errs = append(errs, prefix("bedrock", errs2)...)
	}

	if c.API.Enabled {
		warns2, errs2 := c.API.Config.Validate()
		warns = append(warns, prefix("api", warns2)...)
		errs = append(errs, prefix("api", errs2)...)
	}
	return
}

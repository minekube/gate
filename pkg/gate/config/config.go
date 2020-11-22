package config

import (
	"fmt"
	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/validation"
)

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Editions: Editions{
		Java: Java{
			Enabled: true,
			Config:  jconfig.DefaultConfig,
		},
		Bedrock: Bedrock{
			Enabled: false,
			Config:  bconfig.DefaultConfig,
		},
	},
	HealthService: HealthService{
		Enabled: false,
		Bind:    "0.0.0.0:9090",
	},
}

// Config is the root configuration of Gate.
type Config struct {
	// See Editions struct.
	Editions Editions
	// See HealthService struct.
	HealthService HealthService
}

// Minecraft edition specific configs.
// If multiple editions are enabled, cross-play is activated.
// If no edition is enabled, all will be enabled.
type Editions struct {
	Java    Java
	Bedrock Bedrock
}

// Java edition.
type Java struct {
	Enabled bool
	Config  jconfig.Config
}

// Bedrock edition.
type Bedrock struct {
	Enabled bool
	Config  bconfig.Config
}

// HealthService is a GRPC health probe service for use with Kubernetes pods.
// (https://github.com/grpc-ecosystem/grpc-health-probe)
type HealthService struct {
	Enabled bool
	Bind    string
}

// SetDefaults sets Config defaults to use with Viper.
func SetDefaults(i configutil.SetDefault) {
	i.SetDefault("healthservice.bind", "0.0.0.0:9090")

	i.SetDefault("editions.java.enabled", true)
	i.SetDefault("editions.bedrock.enabled", true)

	// Set Java proxy config defaults
	jconfig.SetDefaults(configutil.SetDefaultFunc(func(key string, value interface{}) {
		// Add prefix
		i.SetDefault("editions.java.config."+key, value)
	}))
}

// Validate validates a Config and all enabled edition configs (Java / Bedrock).
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...interface{}) { errs = append(errs, fmt.Errorf(m, args...)) }
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

	// Validate edition configs
	if c.Editions.Java.Enabled {
		warns2, errs2 := c.Editions.Java.Config.Validate()
		warns = append(warns, prefix("java", warns2)...)
		errs = append(errs, prefix("java", errs2)...)
	}
	if c.Editions.Bedrock.Enabled {
		warns2, errs2 := c.Editions.Java.Config.Validate()
		warns = append(warns, prefix("bedrock", warns2)...)
		errs = append(errs, prefix("bedrock", errs2)...)
	}
	return
}

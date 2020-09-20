package gate

import (
	"fmt"
	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/validation"
)

// Config is a Gate config for reading in files and environment variables with Viper.
type Config struct {
	// Minecraft edition specific configs.
	// If multiple editions are enabled, cross-play is activated.
	// If no edition is enabled, all will be enabled.
	Editions struct {
		Java struct {
			Enabled bool
			Config  jconfig.Config
		}
		Bedrock struct {
			Enabled bool
			Config  bconfig.Config
		}
	}
	// GRPC health probe service for use with Kubernetes pods.
	// (https://github.com/grpc-ecosystem/grpc-health-probe)
	HealthService struct {
		Enabled bool
		Bind    string
	}
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

	return
}

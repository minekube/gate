// Package Gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"fmt"
	"github.com/spf13/viper"
	"go.minekube.com/gate/pkg/bridge"
	"go.minekube.com/gate/pkg/edition"
	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/manager"
)

// Config is a Gate config.
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
}

// Gate manages one or multiple proxy editions (Bedrock & Java).
type Gate struct {
	options *Config
	bridge  *bridge.Bridge
}

// New returns a new Gate instance setup with the given Manager.
func New(mgr manager.Manager, config Config) (g *Gate, err error) {
	config.setDefaults()

	g = &Gate{
		options: &config,
		bridge: &bridge.Bridge{
			Log: mgr.Logger().WithName("bridge"),
		},
	}

	if config.Editions.Java.Enabled {
		g.bridge.JavaProxy, err = jproxy.New(mgr, config.Editions.Java.Config)
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Java, err)
		}
	}
	if config.Editions.Bedrock.Enabled {
		g.bridge.BedrockProxy, err = bproxy.New(mgr, config.Editions.Bedrock.Config)
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Bedrock, err)
		}
	}

	return g, nil
}

func InitViper(v *viper.Viper) {}

func (c *Config) setDefaults() {
	if !c.Editions.Bedrock.Enabled && !c.Editions.Java.Enabled {
		// If all disabled, enable all editions
		c.Editions.Bedrock.Enabled = true
		c.Editions.Java.Enabled = true
	}
}

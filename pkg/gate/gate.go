// Package Gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"fmt"
	"go.minekube.com/gate/pkg/bridge"
	"go.minekube.com/gate/pkg/edition"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/manager"
	"go.minekube.com/gate/pkg/util/errs"
)

// Options are Gate options.
type Options struct {
	// Config requires a valid Gate configuration.
	Config *Config
	// Logger is the logger used for Gate and
	// sub-components like Minecraft edition proxies.
	// If not set, the managers logger is used.
	Logger logr.Logger
}

// New returns a new Gate instance setup with the given Manager.
// The given Options requires a validated Config.
func New(mgr manager.Manager, options Options) (gate *Gate, err error) {
	if options.Config == nil {
		return nil, errs.ErrMissingConfig
	}
	log := options.Logger
	if log == nil {
		log = mgr.Logger().WithName("gate")
	}

	gate = &Gate{
		bridge: &bridge.Bridge{
			Log: log.WithName("bridge"),
		},
	}

	c := options.Config
	if c.Editions.Java.Enabled {
		gate.bridge.JavaProxy, err = jproxy.New(mgr, jproxy.Options{
			Config: &c.Editions.Java.Config,
			Logger: log.WithName("java-proxy"),
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Java, err)
		}
	}
	if c.Editions.Bedrock.Enabled {
		gate.bridge.BedrockProxy, err = bproxy.New(mgr, bproxy.Options{
			Config: &c.Editions.Bedrock.Config,
			Logger: log.WithName("bedrock-proxy"),
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Bedrock, err)
		}
	}

	if c.Editions.Bedrock.Enabled && c.Editions.Java.Enabled {
		// More than once edition was enabled, setup bridge between them
		if err = gate.bridge.Setup(); err != nil {
			return nil, fmt.Errorf("error setting up bridge between proxy editions: %w", err)
		}
	}

	return gate, nil
}

// Gate manages one or multiple proxy editions (Bedrock & Java).
type Gate struct {
	bridge *bridge.Bridge
}

// Java returns the Java edition proxy, or nil if none.
func (g *Gate) Java() *jproxy.Proxy {
	return g.bridge.JavaProxy
}

// Bedrock returns the Bedrock edition proxy, or nil if none.
func (g *Gate) Bedrock() *bproxy.Proxy {
	return g.bridge.BedrockProxy
}

// Package gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"

	"go.minekube.com/gate/pkg/bridge"
	"go.minekube.com/gate/pkg/edition"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/runtime/process"
	connectcfg "go.minekube.com/gate/pkg/util/connectutil/config"
	errors "go.minekube.com/gate/pkg/util/errs"
)

// Options are Gate options.
type Options struct {
	// Config requires a valid Gate configuration.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
}

// New returns a new Gate instance.
// The given Options requires a validated Config.
func New(options Options) (gate *Gate, err error) {
	if options.Config == nil {
		return nil, errors.ErrMissingConfig
	}
	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}

	gate = &Gate{
		proc:   process.New(process.Options{AllOrNothing: true}),
		bridge: &bridge.Bridge{},
	}

	c := options.Config
	if c.Editions.Java.Enabled {
		gate.bridge.JavaProxy, err = jproxy.New(jproxy.Options{
			Config:   &c.Editions.Java.Config,
			EventMgr: eventMgr,
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Java, err)
		}
		if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("java"))
			return gate.bridge.JavaProxy.Start(ctx)
		})); err != nil {
			return nil, err
		}
	}
	if c.Editions.Bedrock.Enabled {
		gate.bridge.BedrockProxy, err = bproxy.New(bproxy.Options{
			Config:   &c.Editions.Bedrock.Config,
			EventMgr: eventMgr,
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Bedrock, err)
		}
		if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("bedrock"))
			return gate.bridge.BedrockProxy.Start(ctx)
		})); err != nil {
			return nil, err
		}
	}

	if c.Editions.Bedrock.Enabled && c.Editions.Java.Enabled {
		// More than one edition was enabled, setup bridge between them
		if err = gate.bridge.Setup(); err != nil {
			return nil, fmt.Errorf("error setting up bridge between proxy editions: %w", err)
		}
	}

	if c.Editions.Java.Enabled { // Currently, only supporting Connect for java edition
		runnable, err := connectcfg.New(
			c.Connect,
			gate.Java(),
		)
		if err != nil {
			return nil, fmt.Errorf("error setting up Connect: %w", err)
		}
		if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("connect"))
			return runnable.Start(ctx)
		})); err != nil {
			return nil, err
		}
	}

	return gate, nil
}

// Gate manages one or multiple proxy editions (Bedrock & Java).
type Gate struct {
	bridge *bridge.Bridge     // The proxies.
	proc   process.Collection // Parallel running proc.
}

// Java returns the Java edition proxy, or nil if none.
func (g *Gate) Java() *jproxy.Proxy {
	return g.bridge.JavaProxy
}

// Bedrock returns the Bedrock edition proxy, or nil if none.
func (g *Gate) Bedrock() *bproxy.Proxy {
	return g.bridge.BedrockProxy
}

// Start starts the Gate instance and all underlying proc.
func (g *Gate) Start(ctx context.Context) error { return g.proc.Start(ctx) }

// Viper is a viper instance initialized
// with defaults for the Config struct.
// It can be used to load in config files.
var Viper = viper.New()

// TODO remove: func init() { config.SetDefaults(Viper) }

// Start is a convenience function to setup and run a Gate instance.
//
// It sets up a Logger, reads in a Config, validates it and sets up
// os signal handling before starting the instance.
//
// The Gate is shutdown on stop channel close or on occurrence of any
// significant error. Config validation warnings are logged but ignored.
func Start(ctx context.Context) error {
	// Clone default config
	cfg := func() config.Config { return config.DefaultConfig }()
	// Load in Gate config
	if err := Viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	log := logr.FromContextOrDiscard(ctx)
	configLog := log.WithName("config")

	// Validate Gate config
	warns, errs := cfg.Validate()
	for _, e := range errs {
		configLog.Info("Config validation error", "error", e.Error())
	}
	for _, w := range warns {
		configLog.Info("Config validation warn", "warn", w.Error())
	}
	if len(errs) != 0 {
		// Shouldn't run Gate with validation errors
		return fmt.Errorf("config validation errors "+
			"(errors: %d, warns: %d), inspect the logs for details",
			len(errs), len(warns))
	}

	// Setup new Gate instance with loaded config.
	gate, err := New(Options{
		Config:   &cfg,
		EventMgr: event.New(log.WithName("event")),
	})
	if err != nil {
		return fmt.Errorf("error creating Gate instance: %w", err)
	}

	// Setup os signal channel to trigger Gate shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer func() { signal.Stop(sig); close(sig) }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer cancel()
		select {
		case <-ctx.Done():
		case s, ok := <-sig:
			if !ok {
				// Sig chan was closed
				return
			}
			log.Info("Received os signal", "signal", s)
		}
	}()

	// Start everything
	return gate.Start(ctx)
}

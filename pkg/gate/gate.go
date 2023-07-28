// Package gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/spf13/viper"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/interrupt"
	"go.uber.org/multierr"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/bridge"
	"go.minekube.com/gate/pkg/edition"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
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
	if !options.Config.Editions.Java.Enabled && !options.Config.Editions.Bedrock.Enabled {
		return nil, fmt.Errorf("no edition enabled, enable at least one Minecraft proxy edition")
	}

	// Require no config validation errors
	warns, errs := options.Config.Validate()
	if err = multierr.Combine(errs...); err != nil {
		return nil, fmt.Errorf("config validation errors "+
			"(errors: %d, warns: %d)", len(errs), len(warns))
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

// Gate is the root holder of various child processes.
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

// Viper is the default viper instance used by Start to load in a config.Config.
var Viper = viper.New()

// StartOption is an option for Start.
type StartOption func(o *startOptions)

type startOptions struct {
	conf                 *config.Config
	autoShutdownOnSignal bool
}

// LoadConfig loads in config.Config from viper.
// It is used by Start with the packages Viper if no WithConfig option is given.
func LoadConfig(v *viper.Viper) (*config.Config, error) {
	// Clone default config
	cfg := func() config.Config { return config.DefaultConfig }()
	// Load in Gate config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}
	// Override Java config by shorter alias
	if !reflect.DeepEqual(cfg.Config, jconfig.DefaultConfig) {
		cfg.Editions.Java.Config = cfg.Config
	}
	return &cfg, nil
}

// WithConfig StartOption for Start.
func WithConfig(c config.Config) StartOption {
	return func(o *startOptions) {
		o.conf = &c
	}
}

// WithAutoShutdownOnSignal StartOption for Start
// that automatically shuts down the Gate instance
// when a shutdown signal is received.
//
// This setting is enabled by default.
func WithAutoShutdownOnSignal(enabled bool) StartOption {
	return func(o *startOptions) {
		o.autoShutdownOnSignal = enabled
	}
}

// Start is a convenience function to set up and run a Gate instance.
//
// It uses the logr.Logger from the provided context, reads in a Config,
// validates it and sets up os signal handling before starting the instance.
//
// The Gate is shutdown when the context is canceled or on occurrence of any
// significant error like severe configuration error or unable to bind to a port.
//
// Config validation warnings are logged but ignored.
func Start(ctx context.Context, opts ...StartOption) error {
	c := &startOptions{
		autoShutdownOnSignal: true,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.conf == nil {
		cfg, err := LoadConfig(Viper)
		if err != nil {
			return err
		}
		c.conf = cfg
	}

	log := logr.FromContextOrDiscard(ctx)
	configLog := log.WithName("config")

	// Validate Gate config
	warns, errs := c.conf.Validate()
	for _, e := range errs {
		configLog.Info("config validation error", "error", e)
	}
	for _, w := range warns {
		configLog.Info("config validation warn", "warn", w)
	}
	if len(errs) != 0 {
		// Shouldn't run Gate with validation errors
		return fmt.Errorf("config validation errors "+
			"(errors: %d, warns: %d), inspect the logs for details",
			len(errs), len(warns))
	}

	// Setup new Gate instance with loaded config.
	gate, err := New(Options{
		Config:   c.conf,
		EventMgr: event.New(event.WithLogger(log.WithName("event"))),
	})
	if err != nil {
		return fmt.Errorf("error creating Gate instance: %w", err)
	}

	// Setup os signal channel to trigger Gate shutdown.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c.autoShutdownOnSignal {
		go func() {
			defer cancel()
			select {
			case <-ctx.Done():
			case s := <-interrupt.Notify(ctx):
				log.Info("Received os signal", "signal", s)
			}
		}()
	}

	// Start everything
	return gate.Start(ctx)
}

// Package Gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/spf13/viper"
	"go.minekube.com/gate/pkg/bridge"
	"go.minekube.com/gate/pkg/edition"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/manager"
	errors "go.minekube.com/gate/pkg/util/errs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
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
		return nil, errors.ErrMissingConfig
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

// Viper is a viper instance initialized with defaults
// for the Config in `pkg/gate/config` package.
// It can be used to load in config files.
var Viper = viper.New()

func init() { SetDefaults(Viper) }

var log = logr.Log.WithName("setup")

// Run is a convenience function to setup and run a Gate instance.
//
// Run sets up a Logger, reads in a Config, validates it, sets up
// os signal handling and creates a Manager to start a new Gate,
//
// The Gate is shutdown on stop channel close or on occurrence of any significant error,
// config validation warnings are logged but ignored.
func Run(stop <-chan struct{}) (err error) {
	// Set logger
	setLogger := func(devMode bool) error {
		zl, err := newZapLogger(devMode)
		if err != nil {
			return fmt.Errorf("error creating zap logger: %w", err)
		}
		logr.SetLogger(zapr.NewLogger(zl))
		return nil
	}
	if err = setLogger(Viper.GetBool("debug")); err != nil {
		return err
	}

	// Load in Gate config
	var cfg Config
	if err := Viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Validate Gate config
	warns, errs := cfg.Validate()
	for _, e := range errs {
		log.Info("Config validation error", "error", e.Error())
	}
	for _, w := range warns {
		log.Info("Config validation warn", "warn", w.Error())
	}
	if len(errs) != 0 {
		// Shouldn't run Gate with validation errors
		return fmt.Errorf("config validation failure (errors: %d, warns: %d), inspect the logs for details",
			len(errs), len(warns))
	}

	// Top-level manager starting all sub-components.
	mgr, err := manager.New(manager.Options{})
	if err != nil {
		return fmt.Errorf("error new runtime manager: %w", err)
	}
	// Setup new Gate with manager and loaded config.
	_, err = New(mgr, Options{Config: &cfg})
	if err != nil {
		return fmt.Errorf("error creating Gate instance: %w", err)
	}

	// Setup os signal channel to trigger Gate shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer func() { signal.Stop(sig); close(sig) }()

	childStop := make(chan struct{})
	go func() {
		defer close(childStop)
		select {
		case <-stop:
		case s, ok := <-sig:
			if !ok {
				// Sig chan was closed
				return
			}
			mgr.Logger().Info("Received a signal", "signal", s)
		}
	}()

	// Start everything
	return mgr.Start(childStop)
}

// newZapLogger returns a new zap logger with a modified production
// or development default config to ensure human readability.
func newZapLogger(dev bool) (l *zap.Logger, err error) {
	var cfg zap.Config
	if dev {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	l, err = cfg.Build()
	if err != nil {
		return nil, err
	}
	return l, nil
}

package gate

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"go.minekube.com/gate/pkg/gate"
	"go.minekube.com/gate/pkg/version"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Execute runs App() with the provided context and calls os.Exit when finished.
func ExecuteContext(ctx context.Context) {
	if err := App().RunContext(ctx, os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

// Execute runs App() and calls os.Exit when finished.
func Execute() {
	if err := App().Run(os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func App() *cli.App {
	app := cli.NewApp()
	app.Name = "gate"
	app.Usage = "Gate is an extensible Minecraft proxy."
	app.Version = version.String()
	app.HideVersion = true // Hide automatic version flags to avoid conflicts
	app.Description = `A high performant & paralleled Minecraft proxy server with
	scalability, flexibility & excelled server version support.

Visit the website https://gate.minekube.com/ for more information.`

	var (
		debug        bool
		configFile   string
		verbosity    int
		showVersion  bool
		noAutoReload bool
	)
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       `config file (default: ./config.yml) Supports: yaml, json, env`,
			EnvVars:     []string{"GATE_CONFIG"},
			Destination: &configFile,
		},
		&cli.BoolFlag{
			Name:        "debug",
			Aliases:     []string{"d"},
			Usage:       "Enable debug mode and highest log verbosity",
			Destination: &debug,
			EnvVars:     []string{"GATE_DEBUG"},
		},
		&cli.IntFlag{
			Name:        "verbosity",
			Aliases:     []string{"v"},
			Usage:       "The higher the verbosity the more logs are shown",
			EnvVars:     []string{"GATE_VERBOSITY"},
			Destination: &verbosity,
		},
		&cli.BoolFlag{
			Name:        "version",
			Aliases:     []string{"V"},
			Usage:       "Show version information",
			Destination: &showVersion,
		},
		&cli.BoolFlag{
			Name:        "no-auto-reload",
			Usage:       "Disable automatic config file reloading",
			Destination: &noAutoReload,
			EnvVars:     []string{"GATE_NO_AUTO_RELOAD"},
		},
	}

	app.Action = func(c *cli.Context) error {
		// Handle version flag (Unix convention: -V for version, -v for verbose)
		if showVersion {
			fmt.Printf("gate version %s\n", version.String())
			return nil
		}

		// Init viper
		v, err := initViper(c, configFile)
		if err != nil {
			return cli.Exit(err, 1)
		}
		// Load config
		cfg, err := gate.LoadConfig(v)
		if err != nil {
			// A config file is only required to exist when explicit config flag was specified.
			// Otherwise, we just use the default config.
			if !(errors.As(err, &viper.ConfigFileNotFoundError{}) || os.IsNotExist(err)) || c.IsSet("config") {
				err = fmt.Errorf("error reading config file %q: %w", v.ConfigFileUsed(), err)
				return cli.Exit(err, 2)
			}
		}

		// Flags overwrite config
		debug = debug || cfg.Config.Debug
		cfg.Config.Debug = debug

		if !c.IsSet("verbosity") && debug {
			verbosity = math.MaxInt8
		}

		// Create or get logger

		var log logr.Logger
		if log, err = logr.FromContext(c.Context); err != nil {
			log, err = newLogger(debug, verbosity)

			if err != nil {
				return cli.Exit(fmt.Errorf("error creating zap logger: %w", err), 1)
			}

			c.Context = logr.NewContext(c.Context, log)
		}

		// Log startup information
		log.Info("starting Gate proxy", "version", version.String())
		log.Info("logging verbosity", "verbosity", verbosity)
		log.Info("using config file", "config", v.ConfigFileUsed())

		// Check if auto reload is disabled (via flag, env var, or config)
		disableAutoReload := noAutoReload || cfg.NoAutoReload

		// Start Gate
		startOpts := []gate.StartOption{gate.WithConfig(*cfg)}
		if !disableAutoReload && v.ConfigFileUsed() != "" {
			startOpts = append(startOpts, gate.WithAutoConfigReload(v.ConfigFileUsed()))
		}
		if err = gate.Start(c.Context, startOpts...); err != nil {
			return cli.Exit(fmt.Errorf("error running Gate: %w", err), 1)
		}
		return nil
	}
	return app
}

func initViper(c *cli.Context, configFile string) (*viper.Viper, error) {
	v := gate.Viper
	if c.IsSet("config") {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
	}
	// Load Environment Variables
	v.SetEnvPrefix("GATE")
	v.AutomaticEnv() // read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind custom environment variables for forwarding secrets
	if err := v.BindEnv("velocitySecret", "GATE_VELOCITY_SECRET"); err != nil {
		return nil, fmt.Errorf("error binding environment variable 'GATE_VELOCITY_SECRET': %w", err)
	}

	if err := v.BindEnv("bungeeGuardSecret", "GATE_BUNGEEGUARD_SECRET"); err != nil {
		return nil, fmt.Errorf("error binding environment variable 'GATE_BUNGEEGUARD_SECRET': %w", err)
	}

	return v, nil
}

// newLogger returns a new zap logger with a modified production
// or development default config to ensure human readability.
func newLogger(debug bool, v int) (l logr.Logger, err error) {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapcore.Level(-v))

	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zl, err := cfg.Build()
	if err != nil {
		return logr.Discard(), err
	}

	return zapr.NewLogger(zl), nil
}

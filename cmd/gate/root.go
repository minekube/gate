package gate

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

// Config discovery used when Gate was not given a config file: a `config.*` file
// in the working directory, which is also what viper's defaults mean.
const (
	defaultConfigName = "config"
	defaultConfigPath = "."
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

	app.Commands = []*cli.Command{
		configCommand(),
	}

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
	// gate.Viper is a gate.NewViper, i.e. it reads config files with delete
	// sharing: the read Gate controls is the delete-sharing one either way, and
	// the read it does not control - viper's own discovery read, which happens
	// when viper's search finds no config file, e.g. in the moment an editor's
	// ReplaceFileW has the name away - cannot refuse the operator's next save on
	// Windows for as long as it is in flight.
	v := gate.Viper
	if c.IsSet("config") {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName(defaultConfigName)
		v.AddConfigPath(defaultConfigPath)
		// Name the file viper's own discovery would pick instead of letting viper
		// discover and read one in a single step: viper reads through
		// afero.ReadFile -> os.ReadFile, a handle that shares read and write but
		// not delete, so on Windows it makes an editor's atomic replacement of
		// config.yml fail with ERROR_SHARING_VIOLATION for as long as the read is
		// in flight - Gate refusing an operator's config save. With the file named
		// here, gate.LoadConfig's read (reload.ReadConfigFile, the delete-sharing
		// reader) is the only reader of it, exactly as when --config is given.
		if file := findDefaultConfigFile(); file != "" {
			v.SetConfigFile(file)
		}
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

// findDefaultConfigFile returns the path of the config file viper's own config
// discovery would resolve for the defaultConfigName/defaultConfigPath settings
// initViper applies, or "" when there is no such file.
//
// Gate resolves the path itself so that it can hand it to viper with
// SetConfigFile, which keeps viper from reading the file content: viper's read
// uses a handle that shares read and write but not delete, and on Windows such a
// handle makes an editor's atomic replacement of config.yml fail with
// ERROR_SHARING_VIOLATION for as long as the read is in flight (see
// reload.ReadConfigFile, the delete-sharing reader Gate reads config content
// with). What viper *finds* stays viper's definition of the search, replicated
// here for the settings above only: each configured path in order, and inside it
// viper's SupportedExts order (config.json, config.toml, config.yaml,
// config.yml, ...), first existing non-directory entry winning. AddConfigPath
// stores the absolute form (absPathify), which is also what viper reports back
// through ConfigFileUsed, so the result is absolute too.
//
// TestDefaultConfigFileDiscoveryMatchesViper pins the replicated search against
// viper's own discovery for a matrix of on-disk layouts, so a change to viper's
// order fails CI instead of changing what Gate loads.
func findDefaultConfigFile() string {
	dir, err := filepath.Abs(defaultConfigPath)
	if err != nil {
		// Mirrors viper's AddConfigPath: a path it cannot absolutize is used as
		// given, and searchInPath joins the file name onto it.
		dir = defaultConfigPath
	}
	for _, ext := range viper.SupportedExts {
		candidate := filepath.Join(dir, defaultConfigName+"."+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	// Viper also tries the config name without an extension, but only once a
	// config type was set - and SetConfigType happens after the config was read
	// (readDecodedConfig), so no file discovered by Gate can be that one.
	return ""
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

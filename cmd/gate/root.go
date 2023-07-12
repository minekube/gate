package gate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"go.minekube.com/gate/pkg/gate"
	"go.minekube.com/gate/pkg/gate/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

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
	app.Description = `A high performant & paralleled Minecraft proxy server with
	scalability, flexibility & excelled server version support.

Visit the website https://developers.minekube.com/gate`

	var (
		debug      bool
		configFile string
		verbosity  int
	)
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage: `config file (default: ./config.yml)
Supports: yaml/yml, json, toml, hcl, ini, prop/properties/props, env/dotenv`,
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
	}
	app.Action = func(c *cli.Context) error {
		// Init viper
		v, err := initViper(c, configFile)
		if err != nil {
			return cli.Exit(err, 1)
		}
		// Load config
		cfg, err := gate.LoadConfig(v)
		if err != nil {
			return cli.Exit(err, 1)
		}

		// Flags overwrite config
		debug = debug || cfg.Editions.Java.Config.Debug
		cfg.Editions.Java.Config.Debug = debug

		if !c.IsSet("verbosity") && debug {
			verbosity = math.MaxInt8
		}

		// Create logger
		log, err := newLogger(debug, verbosity)
		if err != nil {
			return cli.Exit(fmt.Errorf("error creating zap logger: %w", err), 1)
		}
		c.Context = logr.NewContext(c.Context, log)

		log.Info("logging verbosity", "verbosity", verbosity)
		log.Info("using config file", "config", v.ConfigFileUsed())

		// Start Gate
		if err = gate.Start(c.Context, gate.WithConfig(*cfg)); err != nil {
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
	// Read in config.
	cfgCopy := func() config.Config { return config.DefaultConfig }()
	if err := FixedReadInConfig(v, configFile, &cfgCopy); err != nil {
		// A config file is only required to exist when explicit config flag was specified.
		if !(errors.As(err, &viper.ConfigFileNotFoundError{}) || os.IsNotExist(err)) || c.IsSet("config") {
			return nil, fmt.Errorf("error reading config file %q: %w", v.ConfigFileUsed(), err)
		}
	}
	return v, nil
}

// FixedReadInConfig is a workaround for https://github.com/minekube/gate/issues/218#issuecomment-1632800775
func FixedReadInConfig[T any](v *viper.Viper, configFile string, defaultConfig *T) error {
	if configFile == "" || defaultConfig == nil {
		return v.ReadInConfig()
	}

	var (
		unmarshal func([]byte, any) error
		marshal   func(any) ([]byte, error)
	)
	switch path.Ext(configFile) {
	case ".yaml", ".yml":
		unmarshal = yaml.Unmarshal
		marshal = yaml.Marshal
	case ".json":
		unmarshal = json.Unmarshal
		marshal = json.Marshal
	default:
		return fmt.Errorf("unsupported config file format %q", configFile)
	}
	b, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file %q: %w", configFile, err)
	}
	if err = unmarshal(b, defaultConfig); err != nil {
		return fmt.Errorf("error unmarshaling config file %q to %T: %w", configFile, defaultConfig, err)
	}
	if b, err = marshal(defaultConfig); err != nil {
		return fmt.Errorf("error marshaling config file %q: %w", configFile, err)
	}

	return v.ReadConfig(bytes.NewReader(b))
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

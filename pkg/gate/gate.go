// Package gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/edition"
	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	bproxy "go.minekube.com/gate/pkg/edition/bedrock/proxy"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	jconfiglite "go.minekube.com/gate/pkg/edition/java/lite/config"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/internal/otelutil"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/runtime/process"
	connectcfg "go.minekube.com/gate/pkg/util/connectutil/config"
	errorsutil "go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/interrupt"
)

// Options are Gate options.
type Options struct {
	// Config requires a valid Gate configuration.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
	// The config file path for persistence.
	// If none is set, config persistence will be disabled.
	ConfigFilePath string
}

// New returns a new Gate instance.
// The given Options requires a validated Config.
func New(options Options) (gate *Gate, err error) {
	if options.Config == nil {
		return nil, errorsutil.ErrMissingConfig
	}
	// Java is always enabled (embedded config), Bedrock is optional

	// Require no config validation errors
	warns, errs := options.Config.Validate()
	if err = errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("config validation errors "+
			"(errors: %d, warns: %d)", len(errs), len(warns))
	}

	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}
	reload.Map(eventMgr, func(c *config.Config) *jconfig.Config {
		return &c.Config
	})
	reload.Map(eventMgr, func(c *config.Config) *connectcfg.Config {
		return &c.Connect
	})
	// Map Bedrock config reload events
	reload.Map(eventMgr, func(c *config.Config) *bconfig.Config {
		if c.Config.Bedrock.Enabled {
			bedrockConfig := c.Config.Bedrock.ToConfig()
			return &bedrockConfig
		}
		return nil
	})

	gate = &Gate{
		proc: process.New(process.Options{AllOrNothing: true}),
	}

	c := options.Config
	gate.currentConfig.Store(c)
	// Java proxy is always created (embedded config)
	gate.javaProxy, err = jproxy.New(jproxy.Options{
		Config:   &c.Config,
		EventMgr: eventMgr,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Java, err)
	}
	if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
		ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("java"))
		return gate.javaProxy.Start(ctx)
	})); err != nil {
		return nil, err
	}

	if c.Config.Bedrock.Enabled {
		// Convert flattened bedrock config to the original structure
		bedrockConfig := c.Config.Bedrock.ToConfig()
		gate.bedrockProxy, err = bproxy.New(bproxy.Options{
			Config:    &bedrockConfig,
			JavaProxy: gate.javaProxy,
			EventMgr:  eventMgr,
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Bedrock, err)
		}
		if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("bedrock"))
			return gate.bedrockProxy.Start(ctx)
		})); err != nil {
			return nil, err
		}
	}

	if err = setupConnect(gate.proc, c, eventMgr, gate.Java()); err != nil {
		return nil, err
	}

	if err = gate.proc.Add(setupAPI(c, eventMgr, gate.Java(), options.ConfigFilePath)); err != nil {
		return nil, err
	}

	return gate, nil
}

// Gate is the root holder of various child processes.
type Gate struct {
	javaProxy    *jproxy.Proxy      // The Java edition proxy.
	bedrockProxy *bproxy.Proxy      // The Bedrock edition proxy.
	proc         process.Collection // Parallel running proc.

	// currentConfig is an immutable, atomically published runtime snapshot.
	// reloadMu serializes validate/prepare/commit so readers only observe whole snapshots.
	currentConfig atomic.Pointer[config.Config]
	reloadMu      sync.Mutex
}

// LiveConfigResult is the redacted outcome of a file-driven live configuration attempt.
// Code is intentionally a stable category rather than an error detail that could reveal
// configuration contents.
type LiveConfigResult struct {
	Applied          bool
	Unchanged        bool
	CacheInvalidated bool
	Code             string
}

// ApplyLiveConfig validates and atomically publishes the narrow set of source-proven
// live-safe changes: routes in an already-enabled Java Lite configuration. All other
// settings are rejected before any runtime component is changed.
func (g *Gate) ApplyLiveConfig(candidate *config.Config) LiveConfigResult {
	g.reloadMu.Lock()
	defer g.reloadMu.Unlock()

	current := g.currentConfig.Load()
	if candidate == nil {
		return LiveConfigResult{Code: "invalid"}
	}
	if reflect.DeepEqual(current, candidate) {
		return LiveConfigResult{Unchanged: true, Code: "unchanged"}
	}
	if _, errs := candidate.Validate(); len(errs) != 0 {
		return LiveConfigResult{Code: "invalid"}
	}
	if !onlyLiveLiteRoutesChanged(current, candidate) {
		return LiveConfigResult{Code: "unsupported"}
	}
	routes, err := cloneLiveLiteRoutes(candidate.Config.Lite.Routes)
	if err != nil {
		return LiveConfigResult{Code: "prepare_failed"}
	}
	published := *current
	published.Config = current.Config
	published.Config.Lite = current.Config.Lite
	published.Config.Lite.Routes = routes
	if err := g.javaProxy.ApplyLiveConfig(&published.Config); err != nil {
		return LiveConfigResult{Code: "prepare_failed"}
	}
	g.currentConfig.Store(&published)
	return LiveConfigResult{Applied: true, CacheInvalidated: true, Code: "applied"}
}

func cloneLiveLiteRoutes(routes []jconfiglite.Route) ([]jconfiglite.Route, error) {
	encoded, err := json.Marshal(routes)
	if err != nil {
		return nil, err
	}
	var cloned []jconfiglite.Route
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func onlyLiveLiteRoutesChanged(current, candidate *config.Config) bool {
	if current == nil || candidate == nil || !current.Config.Lite.Enabled || !candidate.Config.Lite.Enabled {
		return false
	}
	currentWithoutRoutes := *current
	candidateWithoutRoutes := *candidate
	currentJava := current.Config
	candidateJava := candidate.Config
	currentJava.Lite.Routes = nil
	candidateJava.Lite.Routes = nil
	currentWithoutRoutes.Config = currentJava
	candidateWithoutRoutes.Config = candidateJava
	return reflect.DeepEqual(currentWithoutRoutes, candidateWithoutRoutes)
}

// Java returns the Java edition proxy, or nil if none.
func (g *Gate) Java() *jproxy.Proxy {
	return g.javaProxy
}

// Bedrock returns the Bedrock edition proxy, or nil if none.
func (g *Gate) Bedrock() *bproxy.Proxy {
	return g.bedrockProxy
}

// Start starts the Gate instance and all underlying proc.
func (g *Gate) Start(ctx context.Context) error {
	ctx, span := otel.Tracer("gate").Start(ctx, "gate.Start")
	defer span.End()
	return g.proc.Start(ctx)
}

// Viper is the default viper instance used by Start to load in a config.Config.
var Viper = viper.New()

// StartOption is an option for Start.
type StartOption func(o *startOptions)

type startOptions struct {
	conf                      *config.Config
	autoShutdownOnSignal      bool
	autoConfigReloadWatchPath string
}

// WithConfig is a StartOption for Start
// that uses the provided config.Config.
func WithConfig(c config.Config) StartOption {
	return func(o *startOptions) {
		o.conf = &c
	}
}

// WithAutoShutdownOnSignal is a StartOption for Start
// that automatically shuts down the Gate instance
// when a shutdown signal is received.
//
// This setting is enabled by default.
func WithAutoShutdownOnSignal(enabled bool) StartOption {
	return func(o *startOptions) {
		o.autoShutdownOnSignal = enabled
	}
}

// LoadConfigFunc is a function that loads in a config.Config.
type LoadConfigFunc func() (*config.Config, error)

// WithAutoConfigReload is a StartOption for Start
// that watches the config file and applies supported live changes when a file
// change is detected. Currently, only Lite routes in an already-enabled Lite
// configuration are applied; invalid or unsupported candidates are rejected.
//
// This setting is disabled by default.
func WithAutoConfigReload(path string) StartOption {
	return func(o *startOptions) {
		o.autoConfigReloadWatchPath = path
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
	if err := validateConfig(configLog, c.conf); err != nil {
		return err
	}

	// Setup new Gate instance with loaded config.
	eventMgr := event.New(event.WithLogger(log.WithName("event")))
	gate, err := New(Options{
		Config:         c.conf,
		EventMgr:       eventMgr,
		ConfigFilePath: c.autoConfigReloadWatchPath,
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

	// Initialize OpenTelemetry
	otelShutdown, err := otelutil.Init(ctx)
	if err != nil {
		return fmt.Errorf("error initializing OpenTelemetry: %w", err)
	}
	defer otelShutdown()

	// Setup auto config reload if enabled.
	err = setupAutoConfigReload(
		ctx, configLog, gate,
		c.autoConfigReloadWatchPath, c.conf,
	)
	if err != nil {
		return fmt.Errorf("error setting up auto config reload: %w", err)
	}

	// Start everything
	return gate.Start(ctx)
}

// setupAutoConfigReload sets up auto config reload if enabled.
func setupAutoConfigReload(
	ctx context.Context,
	log logr.Logger,
	gate *Gate,
	path string,
	initialCfg *config.Config,
) error {
	if path == "" {
		return nil // No auto config reload
	}
	log.Info("auto config reload enabled", "path", path)
	_ = initialCfg // Gate owns the immutable initial snapshot.
	// Watch config file for changes
	return reload.Watch(ctx, path, func() error {
		cfg, err := loadLiveConfigCandidate(Viper, path)
		if err != nil {
			return err
		}
		if _, errs := cfg.Validate(); len(errs) != 0 {
			return reload.Reject("invalid")
		}
		result := gate.ApplyLiveConfig(cfg)
		switch result.Code {
		case "applied", "unchanged":
			return nil
		default:
			return reload.Reject(result.Code)
		}
	})
}

// validateConfigFileSyntax rejects incomplete and unknown configuration before
// Viper applies defaults or environment overrides. Errors never leave this
// function so reload diagnostics cannot disclose config contents.
func validateConfigFileSyntax(configPath string) error {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var candidate config.Config
	return decodeConfigStrict(b, path.Ext(configPath), &candidate)
}

func decodeConfigStrict(b []byte, extension string, candidate *config.Config) error {
	switch extension {
	case ".yaml", ".yml":
		decoder := yaml.NewDecoder(bytes.NewReader(b))
		decoder.KnownFields(true)
		if err := decoder.Decode(candidate); err != nil {
			return err
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return errors.New("multiple config documents")
			}
			return err
		}
		return nil
	case ".json":
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(candidate); err != nil {
			return err
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return errors.New("multiple config values")
			}
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported config format")
	}
}

// validateConfig validates the provided config.Config
// and logs any validation errors or warnings.
// If there are any hard errors, it returns an error.
func validateConfig(log logr.Logger, c *config.Config) error {
	// Validate Gate config
	warns, errs := c.Validate()
	for _, e := range errs {
		log.Info("config validation error", "error", e)
	}
	for _, w := range warns {
		log.Info("config validation warn", "warn", w)
	}
	if len(errs) != 0 {
		// Shouldn't run Gate with validation errors
		return fmt.Errorf("config validation errors "+
			"(errors: %d, warns: %d), inspect the logs for details",
			len(errs), len(warns))
	}
	return nil
}

// LoadConfig loads in config.Config from viper.
// It is used by Start with the packages Viper if no WithConfig option is given.
func LoadConfig(v *viper.Viper) (*config.Config, error) {
	cfg := newConfigCandidate()

	// Load in Gate config
	if err := fixedReadInConfig(v, &cfg); err != nil {
		return &cfg, fmt.Errorf("error loading config: %w", err)
	}
	return finishConfigCandidate(v, &cfg), nil
}

// loadLiveConfigCandidate reads and strictly parses one complete file image.
// The candidate is built independently from the current runtime configuration.
func loadLiveConfigCandidate(v *viper.Viper, configPath string) (*config.Config, error) {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil, reload.Reject("read_failed")
	}
	cfg := newConfigCandidate()
	extension := path.Ext(configPath)
	if err := decodeConfigStrict(b, extension, &cfg); err != nil {
		return nil, reload.Reject("parse_failed")
	}
	if err := readDecodedConfig(v, extension, &cfg); err != nil {
		return nil, reload.Reject("parse_failed")
	}
	return finishConfigCandidate(v, &cfg), nil
}

func newConfigCandidate() config.Config {
	encoded, err := json.Marshal(config.DefaultConfig)
	if err != nil {
		panic(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(encoded, &cfg); err != nil {
		panic(err)
	}
	cfg.Config.Servers = make(map[string]string)
	cfg.Config.ForcedHosts = make(map[string][]string)
	return cfg
}

func finishConfigCandidate(v *viper.Viper, cfg *config.Config) *config.Config {
	// Apply environment variable overrides for specific fields
	// This allows environment variables to override config file values
	// Set custom environment variable names for forwarding secrets
	if velocitySecret := v.GetString("velocitySecret"); velocitySecret != "" {
		cfg.Config.Forwarding.VelocitySecret = velocitySecret
	}
	if bungeeGuardSecret := v.GetString("bungeeGuardSecret"); bungeeGuardSecret != "" {
		cfg.Config.Forwarding.BungeeGuardSecret = bungeeGuardSecret
	}

	// Normalize forced hosts keys to lowercase
	if len(cfg.Config.ForcedHosts) > 0 {
		normalizedForcedHosts := make(map[string][]string, len(cfg.Config.ForcedHosts))
		for host, servers := range cfg.Config.ForcedHosts {
			// Convert hostname to lowercase for consistent lookup
			normalizedHost := strings.ToLower(host)
			normalizedForcedHosts[normalizedHost] = servers
		}
		cfg.Config.ForcedHosts = normalizedForcedHosts
	}

	// Java config is now embedded directly in cfg.Config
	return cfg
}

func readDecodedConfig(v *viper.Viper, extension string, cfg *config.Config) error {
	var (
		marshal    func(any) ([]byte, error)
		configType string
	)
	switch extension {
	case ".yaml", ".yml":
		marshal = yaml.Marshal
		configType = "yaml"
	case ".json":
		marshal = json.Marshal
		configType = "json"
	default:
		return fmt.Errorf("unsupported config file format")
	}
	b, err := marshal(cfg)
	if err != nil {
		return err
	}
	v.SetConfigType(configType)
	return v.ReadConfig(bytes.NewReader(b))
}

// Workaround for https://github.com/minekube/gate/issues/218#issuecomment-1632800775
func fixedReadInConfig(v *viper.Viper, defaultConfig *config.Config) error {
	if defaultConfig == nil {
		return v.ReadInConfig()
	}

	configFile := v.ConfigFileUsed()
	if configFile == "" {
		// Try to find config file using Viper's config finder logic
		if err := v.ReadInConfig(); err != nil {
			return err
		}
		configFile = v.ConfigFileUsed()
		if configFile == "" {
			return nil // no config file found
		}
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

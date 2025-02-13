package telemetry

import (
	"context"
	"fmt"

	"go.minekube.com/gate/pkg/edition/java/config"
	gcfg "go.minekube.com/gate/pkg/gate/config"
)

// Init initializes OpenTelemetry with the configured exporters and providers
func Init(ctx context.Context, cfg *gcfg.Config) (cleanup func(), err error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// Initialize telemetry with config
	if cfg.Editions.Java.Config.Telemetry.Metrics.Enabled || cfg.Editions.Java.Config.Telemetry.Tracing.Enabled {
		cleanup, err = initTelemetry(ctx, cfg.Editions.Java.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
		}
		return cleanup, nil
	}

	return func() {}, nil // Return no-op cleanup if telemetry disabled
}

// WithDefaults returns a config with telemetry enabled using reasonable defaults
func WithDefaults(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}

	// Enable telemetry with default configuration if not explicitly configured
	if cfg.Telemetry.Metrics.Enabled || cfg.Telemetry.Tracing.Enabled {
		return cfg // User has explicitly configured telemetry
	}

	// Set default telemetry configuration
	cfg.Telemetry = config.Telemetry{
		Metrics: config.TelemetryMetrics{
			Enabled:         true,
			Endpoint:        "0.0.0.0:8888",
			AnonymousMetrics: true,
			Exporter:        "prometheus",
			Prometheus: struct {
				Path string `yaml:"path" json:"path"`
			}{
				Path: "/metrics",
			},
		},
		Tracing: config.TelemetryTracing{
			Enabled:  false, // Tracing disabled by default
			Endpoint: "localhost:4317",
			Sampler:  "parentbased_always_on",
			Exporter: "stdout",
		},
	}

	return cfg
}
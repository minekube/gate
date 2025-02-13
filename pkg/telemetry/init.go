package telemetry

import (
	"context"
	"fmt"

	gcfg "go.minekube.com/gate/pkg/gate/config"
)

// Init initializes OpenTelemetry with the configured exporters and providers
func Init(ctx context.Context, cfg *gcfg.Config) (cleanup func(), err error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// Initialize telemetry with config
	if cfg.Telemetry.Metrics.Enabled || cfg.Telemetry.Tracing.Enabled {
		cleanup, err = initTelemetry(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
		}
		return cleanup, nil
	}

	return func() {}, nil // Return no-op cleanup if telemetry disabled
}

// WithDefaults returns a config with telemetry enabled using reasonable defaults
func WithDefaults(cfg *gcfg.Config) *gcfg.Config {
	if cfg == nil {
		return nil
	}

	// Enable telemetry with default configuration if not explicitly configured
	if cfg.Telemetry.Metrics.Enabled || cfg.Telemetry.Tracing.Enabled {
		return cfg // User has explicitly configured telemetry
	}

	// Create a copy of the config to avoid modifying the original
	newCfg := *cfg

	// Set default telemetry configuration
	newCfg.Telemetry = gcfg.Telemetry{
		Metrics: gcfg.TelemetryMetrics{
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
		Tracing: gcfg.TelemetryTracing{
			Enabled:  false, // Tracing disabled by default
			Endpoint: "localhost:4317",
			Sampler:  "parentbased_always_on",
			Exporter: "stdout",
		},
	}

	return &newCfg
}
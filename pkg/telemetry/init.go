package telemetry

import (
	"context"

	"go.minekube.com/gate/pkg/gate/config"
)

// Init initializes OpenTelemetry with configuration from environment variables and config.
// It returns a cleanup function and any error encountered.
func Init(ctx context.Context, cfg *config.Config) (func(), error) {
	// Apply default telemetry config first
	cfg = WithDefaults(cfg)

	// Create new telemetry instance
	_, cleanup, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

// WithDefaults returns a copy of the config with default telemetry settings applied.
func WithDefaults(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	// Create a copy to not modify the original
	c := *cfg
	
	// Set default telemetry settings if not configured
	if c.Telemetry.Metrics.Endpoint == "" {
		c.Telemetry.Metrics.Endpoint = "localhost:9464"
	}
	if c.Telemetry.Metrics.Prometheus.Path == "" {
		c.Telemetry.Metrics.Prometheus.Path = "/metrics"
	}
	if c.Telemetry.Tracing.Endpoint == "" {
		c.Telemetry.Tracing.Endpoint = "localhost:4317"
	}
	
	return &c
}
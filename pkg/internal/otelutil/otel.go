// Package otelutil provides OpenTelemetry utilities for Gate
package otelutil

import (
	"context"

	"github.com/honeycombio/otel-config-go/otelconfig"
	"go.minekube.com/gate/pkg/gate/config"
	"go.minekube.com/gate/pkg/telemetry"
)

// Init initializes OpenTelemetry with configuration from environment variables and config
func Init(ctx context.Context, cfg *config.Config) (func(), error) {
	// Apply default telemetry config first
	cfg = telemetry.WithDefaults(cfg)

	// Initialize using honeycomb's otelconfig
	shutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName("gate"),
		otelconfig.WithServiceVersion(telemetry.Version),
	)
	if err != nil {
		return nil, err
	}

	// Create telemetry instance
	_, cleanup, err := telemetry.New(ctx, cfg)
	if err != nil {
		shutdown()
		return nil, err
	}

	return func() {
		cleanup()
		shutdown()
	}, nil
}

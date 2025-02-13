// Package otelutil provides OpenTelemetry utilities for Gate
package otelutil

import (
	"context"

	"go.minekube.com/gate/pkg/telemetry"
	"go.minekube.com/gate/pkg/gate/config"
)

// Init initializes OpenTelemetry with configuration from environment variables and config
func Init(ctx context.Context, cfg *config.Config) (func(), error) {
	return telemetry.Init(ctx, cfg)
}

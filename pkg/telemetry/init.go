package telemetry

import (
	"context"

	"go.minekube.com/gate/pkg/gate/config"
)

// Init initializes OpenTelemetry with configuration from environment variables and config.
// It returns a cleanup function and any error encountered.
func Init(ctx context.Context, cfg *config.Config) (func(), error) {
	// Create new telemetry instance with validated config
	_, cleanup, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}
//go:build musl

package otelutil

import (
	"context"
	"fmt"
	"os"
)

// Init is intentionally minimal for musl builds so the portable Linux binary
// does not depend on host instrumentation packages that pull in libdl.
func Init(context.Context) (func(), error) {
	if os.Getenv("OTEL_METRICS_ENABLED") == "true" || os.Getenv("OTEL_TRACES_ENABLED") == "true" {
		return nil, fmt.Errorf("OpenTelemetry is not available in the musl Linux build; use the standard glibc Linux build for OTEL support")
	}
	return func() {}, nil
}

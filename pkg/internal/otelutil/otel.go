package otelutil

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"go.minekube.com/gate/pkg/version"
)

// Init initializes the OpenTelemetry SDK with the OTLP exporter and the corresponding trace and meter providers.
// It also starts an HTTP server for Prometheus metrics.
func Init(ctx context.Context) (clean func(), err error) {
	fmt.Println(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_INSECURE"))
	// default service name
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "gate"
	}

	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName(serviceName),
		otelconfig.WithServiceVersion(version.String()),
	)
	if err != nil {
		return nil, err
	}

	return func() {
		log := logr.FromContextOrDiscard(ctx).WithName("otel")
		log.Info("shutting down OpenTelemetry, trying to push remaining telemetry data...")
		otelShutdown()
		log.Info("OpenTelemetry shutdown complete")
	}, nil
}

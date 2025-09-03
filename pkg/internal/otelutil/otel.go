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
	// default service name
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "gate"
	}

	log := logr.FromContextOrDiscard(ctx).WithName("otel")

	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName(serviceName),
		otelconfig.WithServiceVersion(version.String()),
		otelconfig.WithLogger(&logger{log}),
		otelconfig.WithMetricsEnabled(os.Getenv("OTEL_METRICS_ENABLED") == "true"),
		otelconfig.WithTracesEnabled(os.Getenv("OTEL_TRACES_ENABLED") == "true"),
	)
	if err != nil {
		return nil, err
	}

	return func() {
		log.Info("shutting down OpenTelemetry, trying to push remaining telemetry data...")
		otelShutdown()
		log.Info("OpenTelemetry shutdown complete")
	}, nil
}

type logger struct {
	logr.Logger
}

var _ otelconfig.Logger = &logger{}

func (l *logger) Debugf(format string, v ...any) {
	l.Logger.V(1).Info(fmt.Sprintf(format, v...))
}

func (l *logger) Fatalf(format string, v ...any) {
	l.Logger.Error(fmt.Errorf(format, v...), "fatal error")
}

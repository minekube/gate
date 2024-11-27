package otelutil

import (
	"os"

	"github.com/honeycombio/otel-config-go/otelconfig"
)

// Init initializes the OpenTelemetry SDK with the OTLP exporter and the corresponding trace and meter providers.
func Init() (clean func(), err error) {
	// default service name
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "gate"
	}

	otelShutdown, err := otelconfig.ConfigureOpenTelemetry(
		otelconfig.WithServiceName(serviceName),
	)
	if err != nil {
		return nil, err
	}
	return otelShutdown, nil
}

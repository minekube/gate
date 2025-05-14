package otelutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/honeycombio/otel-config-go/otelconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

const (
	defaultMetricsPath = "/metrics"
	defaultMetricsAddr = ":9464"
)

// startMetricsServer initializes and starts an HTTP server for Prometheus metrics.
// It returns a function to gracefully shut down the server.
func startMetricsServer(ctx context.Context) (func(), error) {
	enabled := os.Getenv("OTEL_METRICS_ENABLED")
	if enabled != "true" {
		return func() {}, nil
	}
	serverEnabled := os.Getenv("OTEL_METRICS_SERVER_ENABLED")
	if serverEnabled != "true" {
		return func() {}, nil
	}

	metricsPath := os.Getenv("OTEL_METRICS_SERVER_PATH")
	if metricsPath == "" {
		metricsPath = defaultMetricsPath
	}
	metricsAddr := os.Getenv("OTEL_METRICS_SERVER_ADDR")
	if metricsAddr == "" {
		metricsAddr = defaultMetricsAddr
	}

	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	// This provider is set as the global MeterProvider.
	// Application-specific metrics can then be created using otel.Meter()
	// elsewhere and will be exported via this Prometheus setup.
	// The startMetricsServer function itself only sets up the export pipeline.

	mux := http.NewServeMux()
	mux.Handle(metricsPath, promhttp.Handler())

	// Create a ln first to catch errors like "port already in use" early.
	ln, err := net.Listen("tcp", metricsAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics listener on %s: %w", metricsAddr, err)
	}

	server := &http.Server{
		Handler:      mux,
		IdleTimeout:  10 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log := logr.FromContextOrDiscard(ctx)

	go func() {
		log.Info("metrics server listening", "addr", ln.Addr().String(), "path", metricsPath)
		// Use Serve with the listener instead of ListenAndServe
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error(err, "failed to start metrics server")
		}
	}()

	// Return a cleanup function to shut down the server
	return func() {
		log.Info("shutting down metrics server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Error(err, "metrics server shutdown failed")
		}
		_ = ln.Close()
	}, nil
}

// Init initializes the OpenTelemetry SDK with the OTLP exporter and the corresponding trace and meter providers.
// It also starts an HTTP server for Prometheus metrics.
func Init(ctx context.Context) (clean func(), err error) {
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

	metricsServerShutdown, err := startMetricsServer(ctx)
	if err != nil {
		// If metrics server fails, try to clean up OTel config first
		otelShutdown()
		return nil, err
	}

	return func() {
		log := logr.FromContextOrDiscard(ctx).WithName("otel")
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			log.Info("shutting down OpenTelemetry...")
			otelShutdown()
			log.Info("OpenTelemetry shutdown complete")
		}()
		go func() {
			defer wg.Done()
			log.Info("shutting down metrics server...")
			metricsServerShutdown()
			log.Info("metrics server shutdown complete")
		}()
		wg.Wait()
	}, nil
}

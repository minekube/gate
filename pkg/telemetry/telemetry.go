package telemetry

import ( 	
	"context"
	"fmt"
	"net/http"
	"os"
	
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/trace"

	"go.minekube.com/gate/pkg/edition/java/config"
)

const (
	gateVersion = "1.0.0" // TODO: Replace with actual version from build system
)

var (
	tracer     trace.Tracer
	meter      metric.Meter
	tpsValue   float64 // Current TPS value
	gatherer   metric.Float64ObservableUpDownCounter
	playerGauge       metric.Int64UpDownCounter
	connDuration      metric.Float64Histogram
	playerConnections metric.Int64Counter
)

// initTelemetry sets up OpenTelemetry tracing and metrics
func initTelemetry(ctx context.Context, cfg config.Config) (func(), error) {
	// Create shared resource attributes
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("gate"),
			semconv.ServiceVersion(gateVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	var cleanupFuncs []func()

	// Initialize metrics if enabled
	if cfg.Telemetry.Metrics.Enabled {
		metricCleanup, err := initMetrics(ctx, res, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metrics: %w", err)
		}
		cleanupFuncs = append(cleanupFuncs, metricCleanup)
	}

	// Initialize tracing if enabled
	if cfg.Telemetry.Tracing.Enabled {
		tracingCleanup, err := initTracing(ctx, res, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize tracing: %w", err)
		}
		cleanupFuncs = append(cleanupFuncs, tracingCleanup)
	}

	return func() {
		for _, cleanup := range cleanupFuncs {
			cleanup()
		}
	}, nil
}

func initMetrics(ctx context.Context, res *resource.Resource, cfg config.Config) (func(), error) {
	switch cfg.Telemetry.Metrics.Exporter {
	case "prometheus":
		// Create Prometheus exporter
		promExporter, err := prometheus.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
		}

		provider := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(promExporter),
		)
		otel.SetMeterProvider(provider)

		// Start Prometheus HTTP server
		go func() {
			mux := http.NewServeMux()
			mux.Handle(cfg.Telemetry.Metrics.Prometheus.Path, promhttp.Handler())
			if err := http.ListenAndServe(cfg.Telemetry.Metrics.Endpoint, mux); err != nil {
				fmt.Printf("prometheus server error: %v\n", err)
			}
		}()

		return setupMetrics(ctx, provider)

	case "otlp":
		otlpExporter, err := otlpmetricgrpc.New(ctx,
			otlpmetricgrpc.WithEndpoint(cfg.Telemetry.Metrics.Endpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP metrics exporter: %w", err)
		}

		reader := sdkmetric.NewPeriodicReader(otlpExporter)
		provider := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(reader),
		)
		otel.SetMeterProvider(provider)

		return setupMetrics(ctx, provider)

	case "stdout":
		stdoutExporter, err := stdoutmetric.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout metrics exporter: %w", err)
		}

		reader := sdkmetric.NewPeriodicReader(stdoutExporter)
		provider := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(reader),
		)
		otel.SetMeterProvider(provider)

		return setupMetrics(ctx, provider)

	default:
		return nil, fmt.Errorf("unknown metrics exporter: %s", cfg.Telemetry.Metrics.Exporter)
	}
}

func setupMetrics(ctx context.Context, provider *sdkmetric.MeterProvider) (func(), error) {
	meter = provider.Meter("gate")

	// Create metrics
	var err1, err2, err3, err4 error

	// Register TPS callback
	gatherer, err1 = meter.Float64ObservableUpDownCounter(
		"gate.performance.tps",
		metric.WithDescription("Current tick rate"),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			o.Observe(tpsValue)
			return nil
		}),
	)

	playerGauge, err2 = meter.Int64UpDownCounter(
		"gate.players.current",
		metric.WithDescription("Current number of connected players"),
	)

	playerConnections, err3 = meter.Int64Counter(
		"gate.players.total",
		metric.WithDescription("Total number of player connections"),
	)

	connDuration, err4 = meter.Float64Histogram(
		"gate.connection.duration",
		metric.WithDescription("Connection duration in seconds"),
	)

	for _, err := range []error{err1, err2, err3, err4} {
		if err != nil {
			return nil, fmt.Errorf("failed to create metrics: %w", err)
		}
	}

	return func() {
		if err := provider.Shutdown(ctx); err != nil {
			fmt.Printf("failed to shutdown meter provider: %v\n", err)
		}
	}, nil
}

func initTracing(ctx context.Context, res *resource.Resource, cfg config.Config) (func(), error) {
	var exporter sdktrace.SpanExporter
	var err error

	switch cfg.Telemetry.Tracing.Exporter {
	case "otlp":
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(cfg.Telemetry.Tracing.Endpoint),
			otlptracegrpc.WithInsecure(),
		)

	case "stdout":
		exporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint(),
			stdouttrace.WithWriter(os.Stdout),
		)

	case "jaeger":
		return nil, fmt.Errorf("jaeger exporter is deprecated, use OTLP exporter with jaeger collector instead")

	default:
		return nil, fmt.Errorf("unknown tracer type: %s", cfg.Telemetry.Tracing.Exporter)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tracerProvider.Tracer("gate")

	return func() {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			fmt.Printf("failed to shutdown tracer provider: %v\n", err)
		}
	}, nil
}

// RecordPlayerConnection records a new player connection metric
func RecordPlayerConnection(ctx context.Context, username string) {
	if playerConnections != nil {
		playerConnections.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("username", username),
			),
		)
	}
}

// RecordPlayerDisconnection updates metrics when a player disconnects
func RecordPlayerDisconnection(ctx context.Context, username string, duration float64) {
	if connDuration != nil {
		connDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("username", username),
			),
		)
	}
}

// UpdateGathererMetrics updates observable metrics
func UpdateGathererMetrics(_ context.Context, tps float64) {
	tpsValue = tps // Update the TPS value that will be observed by the callback
}

// UpdatePlayerCount updates the current player count
func UpdatePlayerCount(ctx context.Context, delta int64) {
	if playerGauge != nil {
		playerGauge.Add(ctx, delta)
	}
}
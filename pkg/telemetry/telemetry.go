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

	gcfg "go.minekube.com/gate/pkg/gate/config"
)

// Version information set by build flags
var (
	// Version is the current version of Gate.
	// Set using -ldflags "-X go.minekube.com/gate/pkg/telemetry.Version=v1.2.3"
	Version = "dev"
)

// Telemetry holds telemetry instruments for a Gate instance
type Telemetry struct {
	tracer           trace.Tracer
	meter            metric.Meter
	playerGauge      metric.Int64UpDownCounter
	connDuration     metric.Float64Histogram
	playerConnections metric.Int64Counter
}

// New creates a new Telemetry instance for a Gate instance
func New(ctx context.Context, cfg *gcfg.Config) (*Telemetry, func(), error) {
	// Create shared resource attributes
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("gate"),
			semconv.ServiceVersion(Version),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	var cleanupFuncs []func()
	t := &Telemetry{}

	// Initialize metrics if enabled
	if cfg.Telemetry.Metrics.Enabled {
		metricCleanup, err := t.initMetrics(ctx, res, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize metrics: %w", err)
		}
		cleanupFuncs = append(cleanupFuncs, metricCleanup)
	}

	// Initialize tracing if enabled
	if cfg.Telemetry.Tracing.Enabled {
		tracingCleanup, err := t.initTracing(ctx, res, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize tracing: %w", err)
		}
		cleanupFuncs = append(cleanupFuncs, tracingCleanup)
	}

	cleanup := func() {
		for _, cleanup := range cleanupFuncs {
			cleanup()
		}
	}

	return t, cleanup, nil
}

func (t *Telemetry) initMetrics(ctx context.Context, res *resource.Resource, cfg *gcfg.Config) (func(), error) {
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

		return t.setupMetrics(ctx, provider)

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

		return t.setupMetrics(ctx, provider)

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

		return t.setupMetrics(ctx, provider)

	default:
		return nil, fmt.Errorf("unknown metrics exporter: %s", cfg.Telemetry.Metrics.Exporter)
	}
}

func (t *Telemetry) setupMetrics(ctx context.Context, provider *sdkmetric.MeterProvider) (func(), error) {
	t.meter = provider.Meter("gate")

	// Create metrics
	var err1, err2, err3 error

	t.playerGauge, err1 = t.meter.Int64UpDownCounter(
		"gate.players.current",
		metric.WithDescription("Current number of connected players"),
	)

	t.playerConnections, err2 = t.meter.Int64Counter(
		"gate.players.total",
		metric.WithDescription("Total number of player connections"),
	)

	t.connDuration, err3 = t.meter.Float64Histogram(
		"gate.connection.duration",
		metric.WithDescription("Connection duration in seconds"),
	)

	for _, err := range []error{err1, err2, err3} {
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

func (t *Telemetry) initTracing(ctx context.Context, res *resource.Resource, cfg *gcfg.Config) (func(), error) {
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

	t.tracer = tracerProvider.Tracer("gate")

	return func() {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			fmt.Printf("failed to shutdown tracer provider: %v\n", err)
		}
	}, nil
}

// RecordPlayerConnection records a new player connection metric
func (t *Telemetry) RecordPlayerConnection(ctx context.Context, username string) {
	if t.playerConnections != nil {
		t.playerConnections.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("username", username),
			),
		)
	}
}

// RecordPlayerDisconnection updates metrics when a player disconnects
func (t *Telemetry) RecordPlayerDisconnection(ctx context.Context, username string, duration float64) {
	if t.connDuration != nil {
		t.connDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("username", username),
			),
		)
	}
}

// UpdatePlayerCount updates the current player count
func (t *Telemetry) UpdatePlayerCount(ctx context.Context, delta int64) {
	if t.playerGauge != nil {
		t.playerGauge.Add(ctx, delta)
	}
}
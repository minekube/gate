package connection

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MeterObserver exports exactly the Gate connection metric contract through
// OpenTelemetry. It offers no arbitrary attribute API.
type MeterObserver struct {
	events   metric.Int64Counter
	bytes    metric.Int64Counter
	duration metric.Float64Histogram
	active   metric.Int64UpDownCounter
}

func NewMeterObserver(m metric.Meter) (*MeterObserver, error) {
	events, err := m.Int64Counter("gate_connection_events_total", metric.WithUnit("1"), metric.WithDescription("Sanitized Gate connection lifecycle events"))
	if err != nil {
		return nil, err
	}
	bytes, err := m.Int64Counter("gate_network_bytes_total", metric.WithUnit("By"), metric.WithDescription("Raw bytes at bounded Gate connection boundaries"))
	if err != nil {
		return nil, err
	}
	duration, err := m.Float64Histogram("gate_connection_duration_seconds", metric.WithUnit("s"), metric.WithDescription("Lifetime of terminated Gate connections"))
	if err != nil {
		return nil, err
	}
	active, err := m.Int64UpDownCounter("gate_active_connections", metric.WithUnit("1"), metric.WithDescription("Currently active Gate connections"))
	if err != nil {
		return nil, err
	}
	return &MeterObserver{events: events, bytes: bytes, duration: duration, active: active}, nil
}

func eventAttrs(event Event) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("protocol", string(normalizeProtocol(event.Protocol))),
		attribute.String("connection_kind", string(normalizeKind(event.Kind))),
		attribute.String("stage", string(normalizeStage(event.Stage))),
		attribute.String("outcome", string(normalizeOutcome(event.Outcome))),
	)
}

func bytesAttrs(event Event, direction string) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("boundary", string(normalizeBoundary(event.Boundary))),
		attribute.String("protocol", string(normalizeProtocol(event.Protocol))),
		attribute.String("connection_kind", string(normalizeKind(event.Kind))),
		attribute.String("direction", direction),
		attribute.String("stage", string(normalizeStage(event.Stage))),
	)
}

func durationAttrs(event Event) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("protocol", string(normalizeProtocol(event.Protocol))),
		attribute.String("connection_kind", string(normalizeKind(event.Kind))),
		attribute.String("outcome", string(normalizeOutcome(event.Outcome))),
	)
}

func activeAttrs(event Event) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("protocol", string(normalizeProtocol(event.Protocol))),
		attribute.String("connection_kind", string(normalizeKind(event.Kind))),
		attribute.String("stage", string(normalizeStage(event.Stage))),
	)
}

func (m *MeterObserver) Observe(ctx context.Context, event Event) {
	m.events.Add(ctx, 1, eventAttrs(event))
	if event.BytesRead > 0 {
		m.bytes.Add(ctx, event.BytesRead, bytesAttrs(event, "rx"))
	}
	if event.BytesWritten > 0 {
		m.bytes.Add(ctx, event.BytesWritten, bytesAttrs(event, "tx"))
	}
	if event.Terminal {
		m.duration.Record(ctx, event.Duration.Seconds(), durationAttrs(event))
	}
}

func (m *MeterObserver) ObserveActive(ctx context.Context, event Event, delta int64) {
	m.active.Add(ctx, delta, activeAttrs(event))
}

// DefaultObserver fans the OTel observer into the process-wide Prometheus
// registry. Moxy exposes that registry at its private /metrics endpoint and
// Fly scrapes it, so no OTLP collector or user data is required for the cost
// telemetry path.
func DefaultObserver(m metric.Meter) (Observer, error) {
	otelObserver, err := NewMeterObserver(m)
	if err != nil {
		return nil, err
	}
	return &fanoutObserver{observers: []Observer{otelObserver, defaultPrometheusObserver()}}, nil
}

type fanoutObserver struct{ observers []Observer }

func (f *fanoutObserver) Observe(ctx context.Context, event Event) {
	for _, observer := range f.observers {
		observer.Observe(ctx, event)
	}
}

func (f *fanoutObserver) ObserveActive(ctx context.Context, event Event, delta int64) {
	for _, observer := range f.observers {
		if active, ok := observer.(activeObserver); ok {
			active.ObserveActive(ctx, event, delta)
		}
	}
}

type prometheusObserver struct {
	events   *prometheus.CounterVec
	bytes    *prometheus.CounterVec
	duration *prometheus.HistogramVec
	active   *prometheus.GaugeVec
}

var defaultProm struct {
	sync.Once
	observer *prometheusObserver
}

func defaultPrometheusObserver() *prometheusObserver {
	defaultProm.Do(func() { defaultProm.observer = newPrometheusObserver(prometheus.DefaultRegisterer) })
	return defaultProm.observer
}

func newPrometheusObserver(registerer prometheus.Registerer) *prometheusObserver {
	o := &prometheusObserver{
		events:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gate_connection_events_total", Help: "Sanitized Gate connection lifecycle events"}, []string{"protocol", "connection_kind", "stage", "outcome"}),
		bytes:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "gate_network_bytes_total", Help: "Raw bytes at bounded Gate connection boundaries"}, []string{"boundary", "protocol", "connection_kind", "direction", "stage"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "gate_connection_duration_seconds", Help: "Lifetime of terminated Gate connections"}, []string{"protocol", "connection_kind", "outcome"}),
		active:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "gate_active_connections", Help: "Currently active Gate connections"}, []string{"protocol", "connection_kind", "stage"}),
	}
	o.events = registerCounterVec(registerer, o.events)
	o.bytes = registerCounterVec(registerer, o.bytes)
	o.duration = registerHistogramVec(registerer, o.duration)
	o.active = registerGaugeVec(registerer, o.active)
	return o
}

func (o *prometheusObserver) Observe(_ context.Context, event Event) {
	o.events.WithLabelValues(string(normalizeProtocol(event.Protocol)), string(normalizeKind(event.Kind)), string(normalizeStage(event.Stage)), string(normalizeOutcome(event.Outcome))).Inc()
	if event.BytesRead > 0 {
		o.bytes.WithLabelValues(string(normalizeBoundary(event.Boundary)), string(normalizeProtocol(event.Protocol)), string(normalizeKind(event.Kind)), "rx", string(normalizeStage(event.Stage))).Add(float64(event.BytesRead))
	}
	if event.BytesWritten > 0 {
		o.bytes.WithLabelValues(string(normalizeBoundary(event.Boundary)), string(normalizeProtocol(event.Protocol)), string(normalizeKind(event.Kind)), "tx", string(normalizeStage(event.Stage))).Add(float64(event.BytesWritten))
	}
	if event.Terminal {
		o.duration.WithLabelValues(string(normalizeProtocol(event.Protocol)), string(normalizeKind(event.Kind)), string(normalizeOutcome(event.Outcome))).Observe(event.Duration.Seconds())
	}
}

func (o *prometheusObserver) ObserveActive(_ context.Context, event Event, delta int64) {
	o.active.WithLabelValues(string(normalizeProtocol(event.Protocol)), string(normalizeKind(event.Kind)), string(normalizeStage(event.Stage))).Add(float64(delta))
}

func registerCounterVec(r prometheus.Registerer, c *prometheus.CounterVec) *prometheus.CounterVec {
	if err := r.Register(c); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if got, ok := existing.ExistingCollector.(*prometheus.CounterVec); ok {
				return got
			}
		}
		panic(err)
	}
	return c
}
func registerHistogramVec(r prometheus.Registerer, c *prometheus.HistogramVec) *prometheus.HistogramVec {
	if err := r.Register(c); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if got, ok := existing.ExistingCollector.(*prometheus.HistogramVec); ok {
				return got
			}
		}
		panic(err)
	}
	return c
}
func registerGaugeVec(r prometheus.Registerer, c *prometheus.GaugeVec) *prometheus.GaugeVec {
	if err := r.Register(c); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if got, ok := existing.ExistingCollector.(*prometheus.GaugeVec); ok {
				return got
			}
		}
		panic(err)
	}
	return c
}

package connection

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MeterObserver exports the stable gate.connection.v1 schema. The only
// dimensions are the enum values below; it has no arbitrary attribute API.
type MeterObserver struct{ events, bytes metric.Int64Counter }

func NewMeterObserver(m metric.Meter) (*MeterObserver, error) {
	events, err := m.Int64Counter("gate.connection.events", metric.WithUnit("1"), metric.WithDescription("Sanitized Gate connection lifecycle observations"))
	if err != nil {
		return nil, err
	}
	bytes, err := m.Int64Counter("gate.connection.bytes", metric.WithUnit("By"), metric.WithDescription("Raw socket bytes observed at Gate connection boundaries"))
	if err != nil {
		return nil, err
	}
	return &MeterObserver{events: events, bytes: bytes}, nil
}

func (m *MeterObserver) Observe(ctx context.Context, event Event) {
	attrs := metric.WithAttributes(
		attribute.String("gate.connection.schema", "v1"),
		attribute.String("gate.connection.kind", string(normalizeKind(event.Kind))),
		attribute.String("gate.connection.stage", string(normalizeStage(event.Stage))),
		attribute.String("gate.connection.outcome", string(normalizeOutcome(event.Outcome))),
	)
	m.events.Add(ctx, 1, attrs)
	if event.BytesRead > 0 {
		m.bytes.Add(ctx, event.BytesRead, attrs, metric.WithAttributes(attribute.String("gate.connection.direction", "read")))
	}
	if event.BytesWritten > 0 {
		m.bytes.Add(ctx, event.BytesWritten, attrs, metric.WithAttributes(attribute.String("gate.connection.direction", "write")))
	}
}

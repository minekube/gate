package connection

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMeterObserverManualReaderUsesExactBoundedSchema(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	observer, err := NewMeterObserver(provider.Meter("connection-test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := Start(context.Background(), observer)
	// These values model hostile input. None may become a label value.
	session.protocol = Protocol("192.0.2.1")
	session.boundary = Boundary("token=secret")
	session.SetKind(Kind("player=private"))
	session.Observe(ctx, Stage("packet=LoginStart"), Outcome("host=private.example"))
	session.Observe(ctx, Closed, Failed)
	observer.Observe(ctx, Event{Protocol: ProtocolJava, Boundary: ClientEdge, Kind: Login, Stage: Play, Outcome: Success, BytesRead: 7, BytesWritten: 11})

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	wantNames := map[string]bool{
		"gate_connection_events_total":     false,
		"gate_network_bytes_total":         false,
		"gate_connection_duration_seconds": false,
		"gate_active_connections":          false,
	}
	for _, scope := range collected.ScopeMetrics {
		for _, got := range scope.Metrics {
			if _, ok := wantNames[got.Name]; !ok {
				continue
			}
			wantNames[got.Name] = true
			assertBoundedPoints(t, got.Name, got.Data)
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("missing metric %q", name)
		}
	}
}

func TestPrometheusBridgeExposesOnlyCanonicalFamilies(t *testing.T) {
	reg := prometheus.NewRegistry()
	observer := newPrometheusObserver(reg)
	ctx, session := Start(context.Background(), observer)
	session.SetKind(Login)
	session.Observe(ctx, Handshake, OutcomeUnknown)
	observer.Observe(ctx, Event{Protocol: ProtocolJava, Boundary: ClientEdge, Kind: Login, Stage: Play, Outcome: Success, BytesRead: 1, BytesWritten: 1})
	session.Observe(ctx, Closed, Failed)
	got, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"gate_connection_events_total": false, "gate_network_bytes_total": false, "gate_connection_duration_seconds": false, "gate_active_connections": false}
	for _, family := range got {
		if _, ok := want[family.GetName()]; !ok {
			continue
		}
		want[family.GetName()] = true
		for _, point := range family.Metric {
			for _, label := range point.Label {
				if label.GetName() == "remote_addr" || label.GetName() == "identity" || label.GetName() == "error" || label.GetName() == "endpoint" {
					t.Fatalf("unexpected sensitive label %q", label.GetName())
				}
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing Prometheus family %q", name)
		}
	}
}

func TestActiveGaugeKindMigrationBalancesPrometheusAndOTel(t *testing.T) {
	reg := prometheus.NewRegistry()
	prom := newPrometheusObserver(reg)
	ctx, session := Start(context.Background(), prom)
	session.SetKind(Login)
	session.Observe(ctx, Handshake, OutcomeUnknown)
	session.SetKind(Gameplay)
	session.Observe(ctx, Closed, ConnectionClosed)
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range metrics {
		if family.GetName() != "gate_active_connections" {
			continue
		}
		for _, point := range family.Metric {
			if got := point.GetGauge().GetValue(); got != 0 {
				t.Fatalf("Prometheus active gauge leaked %v for %#v", got, point.Label)
			}
		}
	}

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	otelObserver, err := NewMeterObserver(provider.Meter("active-migration"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, session = Start(context.Background(), otelObserver)
	session.SetKind(Login)
	session.Observe(ctx, Handshake, OutcomeUnknown)
	session.SetKind(Gameplay)
	session.Observe(ctx, Closed, ConnectionClosed)
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatal(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, got := range scope.Metrics {
			if got.Name != "gate_active_connections" {
				continue
			}
			sum, ok := got.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("active metric has type %T", got.Data)
			}
			for _, point := range sum.DataPoints {
				if point.Value != 0 {
					t.Fatalf("OTel active gauge leaked %d for %v", point.Value, point.Attributes)
				}
			}
		}
	}
}

func assertBoundedPoints(t *testing.T, name string, data metricdata.Aggregation) {
	t.Helper()
	var points []metricdata.DataPoint[int64]
	switch data := data.(type) {
	case metricdata.Sum[int64]:
		points = data.DataPoints
	case metricdata.Histogram[float64]:
		for _, point := range data.DataPoints {
			assertBoundedAttrs(t, name, point.Attributes)
		}
		return
	default:
		t.Fatalf("%s has unexpected aggregation %T", name, data)
	}
	for _, point := range points {
		assertBoundedAttrs(t, name, point.Attributes)
	}
}

func assertBoundedAttrs(t *testing.T, name string, attrs attribute.Set) {
	t.Helper()
	want := map[string]map[string]bool{
		"protocol":        {"java": true, "bedrock": true, "unknown": true},
		"connection_kind": {"unknown": true, "status": true, "login": true, "transfer": true, "gameplay": true},
		"stage":           {"accepted": true, "handshake": true, "auth": true, "backend": true, "play": true, "closed": true},
		"outcome":         {"unknown": true, "success": true, "failed": true, "timeout": true, "rate_limited": true, "backend_failed": true, "closed": true},
		"boundary":        {"client_edge": true, "connector_tunnel": true, "bedrock_loopback": true, "backend": true},
		"direction":       {"rx": true, "tx": true},
	}
	for _, kv := range attrs.ToSlice() {
		key, value := string(kv.Key), kv.Value.AsString()
		if allowed, ok := want[key]; !ok || !allowed[value] {
			t.Fatalf("%s leaked unbounded attribute %q=%q", name, key, value)
		}
	}
}

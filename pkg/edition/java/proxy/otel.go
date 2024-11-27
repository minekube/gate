package proxy

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var (
	meter  = otel.Meter("java/proxy")
	tracer = otel.Tracer("java/proxy")
)

func (p *Proxy) initMeter() error {
	var err error
	_, err = meter.Int64ObservableGauge(
		"proxy.player_count",
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			o.Observe(int64(p.PlayerCount()))
			return nil
		}),
		metric.WithDescription("The current total player count on the proxy"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}
	return nil
}

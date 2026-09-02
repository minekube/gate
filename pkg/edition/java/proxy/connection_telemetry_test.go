package proxy

import (
	"context"
	"testing"

	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
)

type telemetryEvents []connectiontelemetry.Event

func (e *telemetryEvents) Observe(_ context.Context, event connectiontelemetry.Event) {
	*e = append(*e, event)
}

func TestFullPlayTransitionClassifiesGameplay(t *testing.T) {
	var events telemetryEvents
	ctx, session := connectiontelemetry.Start(context.Background(), &events)
	session.SetKind(connectiontelemetry.Login)
	observeFullGameplay(ctx)
	got := events[len(events)-1]
	if got.Kind != connectiontelemetry.Gameplay || got.Stage != connectiontelemetry.Play || got.Outcome != connectiontelemetry.Success {
		t.Fatalf("full play observation = %#v", got)
	}
}

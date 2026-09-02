package proxy

import (
	"context"
	"errors"
	"net"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/internal/connwrap"
	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
)

type telemetryEvents []connectiontelemetry.Event

func (e *telemetryEvents) Observe(_ context.Context, event connectiontelemetry.Event) {
	*e = append(*e, event)
}

func TestProductionConnectionWrappersDelegateCloseWrite(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "failure", err: errors.New("injected close-write failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &closeWriteSpy{err: tc.err}
			wire := connectiontelemetry.Wrap(base)
			// This is the exact accepted-PROXY + ConnectionEvent wrapper order.
			stack := &connwrap.Conn{Conn: telemetryWireConn{Conn: wire, wire: wire}}
			if err := stack.CloseWrite(); !errors.Is(err, tc.err) {
				t.Fatalf("CloseWrite error = %v, want %v", err, tc.err)
			}
			if base.calls != 1 {
				t.Fatalf("raw CloseWrite calls = %d, want 1", base.calls)
			}
		})
	}
}

type closeWriteSpy struct {
	net.Conn
	err   error
	calls int
}

func (c *closeWriteSpy) CloseWrite() error { c.calls++; return c.err }

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

func TestModernForgeAuthDisconnectTerminatesBeforePlay(t *testing.T) {
	var events telemetryEvents
	ctx, _ := connectiontelemetry.Start(context.Background(), &events)
	// Pre-1.20.2 Modern Forge retains authSessionHandler while its backend
	// relay runs; the helper used by that handler must remain terminal.
	observeAuthDisconnect(ctx)
	got := events[len(events)-1]
	if !got.Terminal || got.Outcome != connectiontelemetry.Failed {
		t.Fatalf("modern-forge auth disconnect telemetry = %#v", got)
	}
}

func TestInitialBackendAttemptClassificationDoesNotDependOnHandler(t *testing.T) {
	for _, tc := range []struct {
		name  string
		type_ phase.ConnectionType
	}{
		{name: "1.20.2-config", type_: phase.Vanilla},
		{name: "pre-1.20.2-modern-forge", type_: phase.ModernForge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events telemetryEvents
			ctx, _ := connectiontelemetry.Start(context.Background(), &events)
			player := &connectedPlayer{MinecraftConn: &testMinecraftConn{ctx: ctx, connType: tc.type_}}
			observeInitialBackendAttempt(player, connectiontelemetry.OutcomeUnknown)
			observeInitialBackendAttempt(player, connectiontelemetry.Timeout)
			got := events[len(events)-1]
			if got.Stage != connectiontelemetry.BackendStage || got.Outcome != connectiontelemetry.Timeout || got.Terminal {
				t.Fatalf("initial backend timeout telemetry = %#v", got)
			}
		})
	}
}

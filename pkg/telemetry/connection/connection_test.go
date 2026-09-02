package connection

import (
	"context"
	"net"
	"sync"
	"testing"
)

type collected struct {
	mu     sync.Mutex
	events []Event
}

func (c *collected) Observe(_ context.Context, event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func TestTrackedConnCountsActualPipeBytesAndSessionDeltasAreMonotone(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	collector := new(collected)
	ctx, session := Start(context.Background(), collector)
	tracked := session.Attach(left)
	go func() { _, _ = right.Write([]byte("abc")) }()
	buf := make([]byte, 2)
	if n, err := tracked.Read(buf); err != nil || n != 2 {
		t.Fatalf("partial read = %d, %v", n, err)
	}
	go func() { _, _ = right.Read(make([]byte, 3)) }()
	if n, err := tracked.Write([]byte("xyz")); err != nil || n != 3 {
		t.Fatalf("write = %d, %v", n, err)
	}
	session.SetKind(Login)
	session.Observe(ctx, Handshake, OutcomeUnknown)
	session.Observe(ctx, Closed, ConnectionClosed)
	if len(collector.events) != 3 {
		t.Fatalf("events = %#v", collector.events)
	}
	if collector.events[1].BytesRead != 2 || collector.events[1].BytesWritten != 3 {
		t.Fatalf("first byte delta = %#v", collector.events[1])
	}
	if collector.events[2].BytesRead != 0 || collector.events[2].BytesWritten != 0 {
		t.Fatalf("repeated observation must have zero byte delta: %#v", collector.events[2])
	}
}

func TestObservationSchemaNormalizesUnknownValuesAndCannotCarryPII(t *testing.T) {
	collector := new(collected)
	ctx, session := Start(context.Background(), collector)
	session.SetKind(Kind("host=private.example"))
	session.Observe(ctx, Stage("packet=LoginStart"), Outcome("secret error text"))
	if got := collector.events[1]; got.Kind != Unknown || got.Stage != Closed || got.Outcome != OutcomeUnknown {
		t.Fatalf("unbounded values escaped normalization: %#v", got)
	}
}

func TestLoopbackBoundaryIsOpaqueAndDoesNotCreateAnotherSession(t *testing.T) {
	ctx, session := Start(context.Background(), nil)
	loopback := WithLoopbackBoundary(ctx)
	if !IsLoopbackBoundary(loopback) || IsLoopbackBoundary(context.Background()) {
		t.Fatal("loopback boundary marker must be explicit and context-local")
	}
	got, ok := FromContext(loopback)
	if !ok || got != session {
		t.Fatal("loopback boundary must preserve the original byte-counting session")
	}
}

func TestBoundedLifecycleCoversStatusLoginTimeoutRateLimitAndBackendFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    Kind
		stage   Stage
		outcome Outcome
	}{
		{"status", Status, Play, Success},
		{"failed-login", Login, Auth, Failed},
		{"login-play", Login, Play, Success},
		{"timeout", Login, Closed, Timeout},
		{"rate-limit", Login, Closed, RateLimited},
		{"backend", Gameplay, Backend, BackendFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := new(collected)
			ctx, session := Start(context.Background(), collector)
			session.SetKind(tc.kind)
			session.Observe(ctx, tc.stage, tc.outcome)
			got := collector.events[1]
			if got.Kind != tc.kind || got.Stage != tc.stage || got.Outcome != tc.outcome {
				t.Fatalf("event = %#v", got)
			}
		})
	}
}

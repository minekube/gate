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

package connection

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type collected struct {
	mu     sync.Mutex
	events []Event
}

func TestTrackedConnCountsPartialBytesReturnedWithError(t *testing.T) {
	tracked := Wrap(&partialConn{})
	buf := make([]byte, 8)
	if n, err := tracked.Read(buf); n != 3 || err != io.EOF {
		t.Fatalf("partial read = %d, %v", n, err)
	}
	if n, err := tracked.Write([]byte("abcd")); n != 2 || err != io.ErrUnexpectedEOF {
		t.Fatalf("partial write = %d, %v", n, err)
	}
	read, written := tracked.Bytes()
	if read != 3 || written != 2 {
		t.Fatalf("partial n+err must be counted, got rx=%d tx=%d", read, written)
	}
}

type partialConn struct{}

func (partialConn) Read(p []byte) (int, error)       { copy(p, "abc"); return 3, io.EOF }
func (partialConn) Write(_ []byte) (int, error)      { return 2, io.ErrUnexpectedEOF }
func (partialConn) Close() error                     { return nil }
func (partialConn) LocalAddr() net.Addr              { return testAddr("local") }
func (partialConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (partialConn) SetDeadline(time.Time) error      { return nil }
func (partialConn) SetReadDeadline(time.Time) error  { return nil }
func (partialConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

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
	if len(collector.events) != 4 {
		t.Fatalf("events = %#v", collector.events)
	}
	if got := collector.events[1]; got.Kind != Unknown || got.Stage != Accepted || got.BytesRead != 2 || got.BytesWritten != 3 {
		t.Fatalf("bytes must flush under their prior snapshot, got %#v", got)
	}
	if collector.events[2].BytesRead != 0 || collector.events[2].BytesWritten != 0 || collector.events[2].Kind != Login || collector.events[2].Stage != Handshake {
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

func TestTerminalOutcomeIsEmittedExactlyOnce(t *testing.T) {
	collector := new(collected)
	ctx, session := Start(context.Background(), collector)
	session.SetKind(Login)
	session.Observe(ctx, Closed, RateLimited)
	// Proxy's unconditional close must not overwrite a more specific quota
	// result, nor create a second terminal event.
	session.Observe(ctx, Closed, ConnectionClosed)
	if len(collector.events) != 2 || collector.events[1].Outcome != RateLimited {
		t.Fatalf("terminal events = %#v", collector.events)
	}
	_, same := Start(ctx, collector)
	if same != session || len(collector.events) != 2 {
		t.Fatal("Start must reuse an existing loopback session without another accepted event")
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
		{"backend", Gameplay, BackendStage, BackendFailed},
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

// Package connection provides the deliberately small, privacy-safe connection
// observation contract used by Gate's protocol front doors.  Its event type is
// intentionally closed: callers cannot attach addresses, identities, packet
// names, errors, or arbitrary attributes.
package connection

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
)

// Kind is the client intention, derived only from the protocol handshake.
type Kind string

const (
	Unknown  Kind = "unknown"
	Status   Kind = "status"
	Login    Kind = "login"
	Transfer Kind = "transfer"
	Gameplay Kind = "gameplay"
)

// Stage is a bounded connection lifecycle stage.
type Stage string

const (
	Accepted  Stage = "accepted"
	Handshake Stage = "handshake"
	Auth      Stage = "auth"
	Backend   Stage = "backend"
	Play      Stage = "play"
	Closed    Stage = "closed"
)

// Outcome is intentionally bounded.  Do not put error strings in telemetry.
type Outcome string

const (
	OutcomeUnknown   Outcome = "unknown"
	Success          Outcome = "success"
	Failed           Outcome = "failed"
	Timeout          Outcome = "timeout"
	RateLimited      Outcome = "rate_limited"
	BackendFailed    Outcome = "backend_failed"
	ConnectionClosed Outcome = "closed"
)

// Event is the complete public connection observation schema. It contains no
// string field that can accidentally carry PII or high-cardinality values.
type Event struct {
	Kind         Kind
	Stage        Stage
	Outcome      Outcome
	BytesRead    int64
	BytesWritten int64
}

// Observer receives sanitized connection observations.
type Observer interface{ Observe(context.Context, Event) }

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(context.Context, Event)

func (f ObserverFunc) Observe(ctx context.Context, event Event) { f(ctx, event) }

// TrackedConn counts bytes actually accepted by the socket, including partial
// reads and writes. It deliberately counts n rather than len(p).
type TrackedConn struct {
	net.Conn
	read    atomic.Int64
	written atomic.Int64
}

func Wrap(conn net.Conn) *TrackedConn { return &TrackedConn{Conn: conn} }

func (c *TrackedConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.read.Add(int64(n))
	}
	return n, err
}

func (c *TrackedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.written.Add(int64(n))
	}
	return n, err
}

func (c *TrackedConn) Bytes() (read, written int64) { return c.read.Load(), c.written.Load() }

type contextKey struct{}
type loopbackContextKey struct{}

// WithLoopbackBoundary marks a trusted in-process Bedrock-to-Java handoff.
// It carries no identity, address, or protocol payload and lets the Java front
// door retain one Session/TrackedConn around the proxy-protocol wrapper rather
// than creating a second byte counter at the Geyser boundary.
func WithLoopbackBoundary(ctx context.Context) context.Context {
	return context.WithValue(ctx, loopbackContextKey{}, struct{}{})
}

// IsLoopbackBoundary reports whether ctx came from the in-process Bedrock
// loopback handoff. It is deliberately not exported as a metric dimension.
func IsLoopbackBoundary(ctx context.Context) bool {
	_, ok := ctx.Value(loopbackContextKey{}).(struct{})
	return ok
}

// Session owns one accepted socket's sanitized observation lifecycle.
type Session struct {
	observer              Observer
	mu                    sync.Mutex
	conn                  *TrackedConn
	kind                  Kind
	lastRead, lastWritten int64
}

// Start records acceptance and adds the session to ctx. Attach must be called
// before protocol I/O to make byte accounting active.
func Start(ctx context.Context, observer Observer) (context.Context, *Session) {
	s := &Session{observer: observer, kind: Unknown}
	ctx = context.WithValue(ctx, contextKey{}, s)
	s.Observe(ctx, Accepted, OutcomeUnknown)
	return ctx, s
}

func FromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(contextKey{}).(*Session)
	return s, ok
}

// Attach returns a counting wrapper. It may be called once, before I/O.
func (s *Session) Attach(conn net.Conn) net.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		s.conn = Wrap(conn)
	}
	return s.conn
}

func (s *Session) SetKind(kind Kind) {
	s.mu.Lock()
	s.kind = normalizeKind(kind)
	s.mu.Unlock()
}

func (s *Session) Observe(ctx context.Context, stage Stage, outcome Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observer == nil {
		return
	}
	var read, written int64
	if s.conn != nil {
		read, written = s.conn.Bytes()
	}
	// A meter treats byte values as deltas at every lifecycle event, so repeated
	// observations cannot make its counters go backwards or double count.
	deltaRead, deltaWritten := read-s.lastRead, written-s.lastWritten
	s.lastRead, s.lastWritten = read, written
	s.observer.Observe(ctx, Event{Kind: normalizeKind(s.kind), Stage: normalizeStage(stage), Outcome: normalizeOutcome(outcome), BytesRead: deltaRead, BytesWritten: deltaWritten})
}

func normalizeKind(v Kind) Kind {
	switch v {
	case Status, Login, Transfer, Gameplay:
		return v
	default:
		return Unknown
	}
}
func normalizeStage(v Stage) Stage {
	switch v {
	case Accepted, Handshake, Auth, Backend, Play, Closed:
		return v
	default:
		return Closed
	}
}
func normalizeOutcome(v Outcome) Outcome {
	switch v {
	case Success, Failed, Timeout, RateLimited, BackendFailed, ConnectionClosed:
		return v
	default:
		return OutcomeUnknown
	}
}

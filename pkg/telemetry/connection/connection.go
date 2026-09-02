// Package connection provides the deliberately small, privacy-safe connection
// observation contract used by Gate's protocol front doors. Its event type is
// closed: callers cannot attach addresses, identities, packet names, errors,
// or arbitrary attributes.
package connection

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Protocol identifies the client protocol at a bounded telemetry boundary.
type Protocol string

const (
	ProtocolUnknown Protocol = "unknown"
	ProtocolJava    Protocol = "java"
	ProtocolBedrock Protocol = "bedrock"
)

// Boundary identifies where bytes crossed a bounded component boundary.
type Boundary string

const (
	ClientEdge      Boundary = "client_edge"
	ConnectorTunnel Boundary = "connector_tunnel"
	BedrockLoopback Boundary = "bedrock_loopback"
	Backend         Boundary = "backend"
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
	Accepted     Stage = "accepted"
	Handshake    Stage = "handshake"
	Auth         Stage = "auth"
	BackendStage Stage = "backend"
	Play         Stage = "play"
	Closed       Stage = "closed"
)

// Outcome is intentionally bounded. Do not put error strings in telemetry.
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
// field that can accidentally carry PII or high-cardinality values.
type Event struct {
	Protocol     Protocol
	Boundary     Boundary
	Kind         Kind
	Stage        Stage
	Outcome      Outcome
	BytesRead    int64
	BytesWritten int64
	Duration     time.Duration
	Terminal     bool
}

// Observer receives sanitized connection observations.
type Observer interface{ Observe(context.Context, Event) }

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(context.Context, Event)

func (f ObserverFunc) Observe(ctx context.Context, event Event) { f(ctx, event) }

// activeObserver is intentionally internal: a Session is the only type that
// can change active-connections state, so callers cannot make arbitrary gauges.
type activeObserver interface {
	ObserveActive(context.Context, Event, int64)
}

// byteObserver receives a snapshot flush without turning it into another
// lifecycle event. Implementations must only update byte instruments.
type byteObserver interface {
	ObserveBytes(context.Context, Event)
}

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

// CloseWrite preserves TCP half-close support through the production byte
// wrapper. Callers can distinguish unsupported/failed half-closes from a
// successful one and fall back to a full close.
func (c *TrackedConn) CloseWrite() error {
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return net.ErrClosed
}

type contextKey struct{}
type loopbackContextKey struct{}

// WithLoopbackBoundary marks a trusted in-process Bedrock-to-Java handoff. It
// carries no identity, address, or protocol payload and makes the Java front
// door account it as Bedrock loopback rather than a second client-edge socket.
func WithLoopbackBoundary(ctx context.Context) context.Context {
	return context.WithValue(ctx, loopbackContextKey{}, struct{}{})
}

// IsLoopbackBoundary reports whether ctx came from the in-process Bedrock
// loopback handoff. It is deliberately not a caller-provided metric attribute.
func IsLoopbackBoundary(ctx context.Context) bool {
	_, ok := ctx.Value(loopbackContextKey{}).(struct{})
	return ok
}

// Session owns one accepted socket's sanitized observation lifecycle.
type Session struct {
	observer              Observer
	mu                    sync.Mutex
	conn                  *TrackedConn
	protocol              Protocol
	boundary              Boundary
	kind                  Kind
	stage                 Stage
	active                bool
	terminal              bool
	started               time.Time
	lastRead, lastWritten int64
	activeCtx             context.Context
}

// Start records acceptance and adds the session to ctx. Attach must be called
// before protocol I/O to make byte accounting active.
func Start(ctx context.Context, observer Observer) (context.Context, *Session) {
	if existing, ok := FromContext(ctx); ok {
		return ctx, existing
	}
	protocol, boundary := ProtocolJava, ClientEdge
	if IsLoopbackBoundary(ctx) {
		protocol, boundary = ProtocolBedrock, BedrockLoopback
	}
	s := &Session{observer: observer, protocol: protocol, boundary: boundary, kind: Unknown, stage: Accepted, started: time.Now()}
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
		// A protocol decoder may wrap the transport (for example to consume a
		// PROXY header). Keep the counter at the wire boundary when it supplies
		// one, while returning the decoder-facing connection unchanged.
		if carrier, ok := conn.(interface{ TelemetryWireConn() *TrackedConn }); ok {
			if wire := carrier.TelemetryWireConn(); wire != nil {
				s.conn = wire
				return conn
			}
		}
		if tracked, ok := conn.(*TrackedConn); ok {
			s.conn = tracked
			return conn
		}
		s.conn = Wrap(conn)
	}
	return s.conn
}

func (s *Session) SetKind(kind Kind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind = normalizeKind(kind)
	if s.kind == kind {
		return
	}
	// Bytes accrued before reclassification belong to the prior lifecycle
	// snapshot, never to the newly inferred kind.
	s.flushLocked(s.activeCtx)
	if s.active {
		if active, ok := s.observer.(activeObserver); ok {
			old := Event{Protocol: s.protocol, Boundary: s.boundary, Kind: s.kind, Stage: s.stage}
			active.ObserveActive(s.activeCtx, old, -1)
			old.Kind = kind
			active.ObserveActive(s.activeCtx, old, 1)
		}
	}
	s.kind = kind
}

func (s *Session) Observe(ctx context.Context, stage Stage, outcome Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observer == nil {
		return
	}
	stage, outcome = normalizeStage(stage), normalizeOutcome(outcome)
	if stage == Closed && s.terminal {
		return
	}
	if s.activeCtx == nil {
		s.activeCtx = ctx
	}
	// A stage boundary is also a byte-label boundary. Flush before changing the
	// snapshot so handshake/auth/play bytes retain the stage in which they were
	// observed; the lifecycle event below then starts the new snapshot at zero.
	if stage != s.stage {
		s.flushLocked(ctx)
	}
	terminal := stage == Closed
	event := Event{
		Protocol: s.protocol, Boundary: s.boundary, Kind: normalizeKind(s.kind),
		Stage: stage, Outcome: outcome,
		Terminal: terminal,
	}
	if stage == s.stage {
		event.BytesRead, event.BytesWritten = s.byteDeltaLocked()
	}
	if terminal {
		event.Duration = time.Since(s.started)
	}
	if active, ok := s.observer.(activeObserver); ok {
		if s.active && (terminal || s.stage != stage || s.kind != event.Kind) {
			active.ObserveActive(ctx, Event{Protocol: s.protocol, Boundary: s.boundary, Kind: s.kind, Stage: s.stage}, -1)
			s.active = false
		}
		if !terminal && !s.active {
			active.ObserveActive(ctx, event, 1)
			s.active = true
		}
	}
	s.kind, s.stage = event.Kind, stage
	if terminal {
		s.terminal = true
	}
	s.observer.Observe(ctx, event)
}

// flushLocked records a byte delta under the current lifecycle snapshot. The
// caller holds s.mu. It intentionally emits no zero-byte event.
func (s *Session) flushLocked(ctx context.Context) {
	read, written := s.byteDeltaLocked()
	if (read == 0 && written == 0) || s.observer == nil {
		return
	}
	event := Event{
		Protocol: s.protocol, Boundary: s.boundary, Kind: normalizeKind(s.kind),
		Stage: s.stage, Outcome: OutcomeUnknown, BytesRead: read, BytesWritten: written,
	}
	if observer, ok := s.observer.(byteObserver); ok {
		observer.ObserveBytes(ctx, event)
		return
	}
	// Custom observers without the optional byte-only path retain the original
	// Event contract; built-in Prometheus and OTel observers never take this
	// branch, so their lifecycle counters remain exact.
	s.observer.Observe(ctx, event)
}

// byteDeltaLocked advances the byte snapshot and returns the delta. The caller
// holds s.mu.
func (s *Session) byteDeltaLocked() (read, written int64) {
	if s.conn == nil {
		return 0, 0
	}
	read, written = s.conn.Bytes()
	read, written = read-s.lastRead, written-s.lastWritten
	s.lastRead += read
	s.lastWritten += written
	return read, written
}

func normalizeProtocol(v Protocol) Protocol {
	if v == ProtocolBedrock || v == ProtocolJava {
		return v
	}
	return ProtocolUnknown
}
func normalizeBoundary(v Boundary) Boundary {
	switch v {
	case ClientEdge, ConnectorTunnel, BedrockLoopback, Backend:
		return v
	}
	return ClientEdge
}
func normalizeKind(v Kind) Kind {
	switch v {
	case Status, Login, Transfer, Gameplay:
		return v
	}
	return Unknown
}
func normalizeStage(v Stage) Stage {
	switch v {
	case Accepted, Handshake, Auth, BackendStage, Play, Closed:
		return v
	}
	return Closed
}
func normalizeOutcome(v Outcome) Outcome {
	switch v {
	case Success, Failed, Timeout, RateLimited, BackendFailed, ConnectionClosed:
		return v
	}
	return OutcomeUnknown
}

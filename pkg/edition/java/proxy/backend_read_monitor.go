package proxy

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// Stages of a backend connection that Gate can be waiting on. They name how far
// the connection got, which is what turns "the player was disconnected" into an
// actionable log line: a backend that accepts the TCP connection and never
// speaks is otherwise invisible in Gate's log, and the read timeout that
// eventually fires names neither the backend nor what Gate was waiting for.
const (
	backendStageHandshake     = "handshake"
	backendStageLogin         = "login"
	backendStageConfiguration = "configuration"
	backendStagePlay          = "play"
)

// backendReadMonitor observes the connection to a backend server: how many bytes
// the backend sent, when it last sent one, and why the last read failed.
//
// It only observes. Reads, deadlines and errors pass through unchanged, so
// wrapping a backend connection can never change how Gate talks to a backend -
// it only lets the session handlers name the failure afterwards. Observing bytes
// rather than only the final read error matters: when a stalled join is torn
// down, the socket read that was pending usually fails with "use of closed
// network connection" (Gate closing it) rather than with a read timeout, and
// that must not hide the fact that the backend never answered.
type backendReadMonitor struct {
	net.Conn

	mu         sync.Mutex
	read       int64
	lastReadAt time.Time
	err        error
	timedOut   bool
}

func (c *backendReadMonitor) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.mu.Lock()
		c.read += int64(n)
		c.lastReadAt = time.Now()
		c.mu.Unlock()
	}
	if err != nil {
		var netErr net.Error
		c.mu.Lock()
		c.err = err
		c.timedOut = errors.As(err, &netErr) && netErr.Timeout()
		c.mu.Unlock()
	}
	return n, err
}

// setBackendStage records how far this backend connection has progressed.
func (s *serverConnection) setBackendStage(stage string) {
	s.mu.Lock()
	s.backendStage = stage
	s.mu.Unlock()
}

// backendStall describes a backend connection and how quiet the backend was.
type backendStall struct {
	backend     string
	address     string
	stage       string
	bytesRead   int64
	silentFor   time.Duration
	readTimeout time.Duration
	err         error
}

// backendStallSnapshot returns the current stall picture of this backend
// connection. It reports false when Gate never got a backend socket to observe.
func (s *serverConnection) backendStallSnapshot() (backendStall, bool) {
	if s == nil || s.server == nil {
		return backendStall{}, false
	}
	s.mu.RLock()
	monitor := s.backendRead
	stage := s.backendStage
	established := s.backendConnectedAt
	s.mu.RUnlock()
	if monitor == nil {
		return backendStall{}, false
	}

	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	stall := backendStall{
		backend:     s.server.info.Name(),
		address:     s.server.info.Addr().String(),
		stage:       stage,
		bytesRead:   monitor.read,
		err:         monitor.err,
		readTimeout: s.backendReadTimeout(),
	}
	lastRead := monitor.lastReadAt
	if lastRead.IsZero() {
		lastRead = established
	}
	stall.silentFor = time.Since(lastRead)
	return stall, true
}

// backendReadTimeout is the read timeout an operator configured, and the
// deadline the connection layer installs on the backend connection: the two are
// the same value, so the diagnostics below report what the operator configured
// and what was actually enforced.
func (s *serverConnection) backendReadTimeout() time.Duration {
	if s == nil || s.player == nil {
		return 0
	}
	return time.Duration(s.config().ReadTimeout)
}

// reportStalledBackend logs why a backend connection ended before the backend
// answered.
func (s *serverConnection) reportStalledBackend(log logr.Logger) {
	if stall, ok := s.backendStallSnapshot(); ok {
		logStalledBackend(log, stall)
	}
}

// bridgeDialFailed reports why the translation bridge could not reach the
// dynamic backend this connection was dialled for, if this join went through a
// bridge and the bridge could not dial it.
//
// The bridge's own failure names the backend, the bridge address the hop dials
// and the stage, which is strictly more useful than the generic
// closed-connection message every other backend teardown gets - and it is the
// only report of a dial that never answered at all.
func (s *serverConnection) bridgeDialFailed() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	failure := s.bridgeDialFailure
	s.mu.RUnlock()
	if failure == nil {
		return nil
	}
	return failure()
}

// logStalledBackend logs the two shapes of a stalled backend that used to be
// silent:
//
//   - the backend accepted the connection and never sent a single packet, and
//   - the backend sent something and then went quiet for at least the
//     configured read timeout.
//
// A backend that answered and then failed mid-conversation is not reported: that
// failure belongs to the phase that was active, and reporting it here would turn
// every backend restart into an error line.
func logStalledBackend(log logr.Logger, stall backendStall) {
	neverAnswered, wentQuiet := stalledBackendShape(stall)
	switch {
	case neverAnswered:
		log.Error(stall.err, "backend server never answered: the backend connection ended without it sending a single packet",
			stallAttrs(stall, "waited", stall.silentFor)...)
	case wentQuiet:
		log.Error(stall.err, "backend server stopped sending packets and the backend connection ended",
			stallAttrs(stall, "silentFor", stall.silentFor, "bytesRead", stall.bytesRead)...)
	}
}

// stallAttrs builds the shared key/value part of a stall diagnostic.
func stallAttrs(stall backendStall, extra ...any) []any {
	attrs := []any{
		"backend", stall.backend,
		"address", stall.address,
		"stage", stall.stage,
	}
	attrs = append(attrs, extra...)
	attrs = append(attrs,
		"readTimeout", stall.readTimeout.String(),
		"hint", "check that the configured address really is a Minecraft backend (or a proxy forwarding to one) and that it is not stalled",
	)
	return attrs
}

// stalledBackendShape decides whether a backend connection was stalled, and if
// so which shape it was.
//
// Neither shape looks at the read error: when Gate tears a stalled join down,
// the pending backend read usually fails with "use of closed network connection"
// instead of a read timeout, and the diagnostic must survive that.
func stalledBackendShape(stall backendStall) (neverAnswered, wentQuiet bool) {
	if stall.bytesRead == 0 {
		return true, false
	}
	return false, stall.readTimeout > 0 && stall.silentFor >= stall.readTimeout
}

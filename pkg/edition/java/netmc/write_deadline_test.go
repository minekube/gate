package netmc

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"

	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/gate/proto"
)

// logLines collects everything a production-verbosity (V(0)) logger emits.
type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) log(prefix, args string) {
	l.mu.Lock()
	l.lines = append(l.lines, strings.TrimSpace(prefix+" "+args))
	l.mu.Unlock()
}

func (l *logLines) first(want string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for {
		l.mu.Lock()
		for _, line := range l.lines {
			if strings.Contains(line, want) {
				l.mu.Unlock()
				return line, true
			}
		}
		l.mu.Unlock()
		if time.Now().After(deadline) {
			return "", false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (l *logLines) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// TestWriteDeadlineCloseIsLoggedAtProductionVerbosity is the visibility the
// split write budget exists for: when a flush cannot drain within writeTimeout
// Gate closes the connection, and that close has to be countable in production.
// The line used to be V(1)-only (debug), so a write-deadline close was
// invisible at the verbosity Gate actually runs at.
func TestWriteDeadlineCloseIsLoggedAtProductionVerbosity(t *testing.T) {
	const writeTimeout = 80 * time.Millisecond

	// A peer that never reads: every write to it blocks until the deadline.
	peer, proxySide := net.Pipe()
	defer peer.Close()
	defer proxySide.Close()

	logs := &logLines{}
	log := funcr.New(logs.log, funcr.Options{Verbosity: 0})

	conn, _ := NewMinecraftConn(
		logr.NewContext(context.Background(), log),
		proxySide, proto.ClientBound,
		30*time.Second, // readTimeout: not the deadline under test
		writeTimeout,
		-1, nil,
	)
	conn.SetActiveSessionHandler(state.Play, nopSessionHandler{})

	// The writer installs the deadline on flush, so seed one before writing a
	// payload that cannot fit in the write buffer.
	if err := conn.Flush(); err != nil {
		t.Fatalf("seeding the write deadline: %v", err)
	}
	if err := conn.Write(bytes.Repeat([]byte{0x01}, 64<<10)); err == nil {
		t.Fatal("a write to a peer that never reads must fail on the write deadline")
	}
	if !Closed(conn) {
		t.Fatal("a write-deadline failure must close the connection")
	}

	line, ok := logs.first("write deadline", 2*time.Second)
	if !ok {
		t.Fatalf("no write-deadline close was logged at production verbosity (V(0)); got:\n%s",
			strings.Join(logs.all(), "\n"))
	}
	for _, want := range []string{"closeKind", "writeTimeout", writeTimeout.String()} {
		if !strings.Contains(line, want) {
			t.Errorf("write-deadline line is missing %q, so the close is not attributable:\n%s", want, line)
		}
	}
}

// TestOrdinaryWriteErrorIsNotReportedAsADeadline is the control: an error that
// is not the write deadline (here: a connection the peer already closed) must
// not be counted as a write-deadline close, or the new line stops meaning
// anything.
func TestOrdinaryWriteErrorIsNotReportedAsADeadline(t *testing.T) {
	peer, proxySide := net.Pipe()

	logs := &logLines{}
	log := funcr.New(logs.log, funcr.Options{Verbosity: 0})

	conn, _ := NewMinecraftConn(
		logr.NewContext(context.Background(), log),
		proxySide, proto.ClientBound,
		30*time.Second, 10*time.Second, -1, nil,
	)
	conn.SetActiveSessionHandler(state.Play, nopSessionHandler{})

	if err := conn.Flush(); err != nil {
		t.Fatalf("seeding the write deadline: %v", err)
	}
	_ = peer.Close() // the peer goes away, instead of going quiet

	_ = conn.Write(bytes.Repeat([]byte{0x01}, 64<<10))

	if line, ok := logs.first("write deadline", 200*time.Millisecond); ok {
		t.Fatalf("a closed peer was reported as a write-deadline close:\n%s", line)
	}
}

// nopSessionHandler satisfies SessionHandler so a connection can be closed in
// the tests above without wiring a real session.
type nopSessionHandler struct{}

func (nopSessionHandler) HandlePacket(*proto.PacketContext) {}
func (nopSessionHandler) Disconnected()                     {}
func (nopSessionHandler) Activated()                        {}
func (nopSessionHandler) Deactivated()                      {}

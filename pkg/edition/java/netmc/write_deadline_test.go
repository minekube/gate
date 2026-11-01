package netmc

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
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

// TestReadDeadlineCloseIsLoggedAtProductionVerbosity is the silence half: a peer
// that stops sending for readTimeout is closed by Gate's read deadline, and that
// close has to be countable too. The decoder wraps its read errors as silent
// errors (so an untrusted peer cannot flood the log), which used to swallow this
// one as well, so the close ended as an unrelated decode error.
func TestReadDeadlineCloseIsLoggedAtProductionVerbosity(t *testing.T) {
	const readTimeout = 80 * time.Millisecond

	peer, proxySide := net.Pipe() // the peer never sends a byte
	defer peer.Close()
	defer proxySide.Close()

	logs := &logLines{}
	log := funcr.New(logs.log, funcr.Options{Verbosity: 0})

	conn, readLoop := NewMinecraftConn(
		logr.NewContext(context.Background(), log),
		proxySide, proto.ClientBound,
		readTimeout, 30*time.Second, -1, nil,
	)
	conn.SetActiveSessionHandler(state.Play, nopSessionHandler{})
	go readLoop()

	deadline := time.Now().Add(2 * time.Second)
	for !Closed(conn) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !Closed(conn) {
		t.Fatal("the read deadline did not close the connection")
	}

	line, ok := logs.first("read deadline", 2*time.Second)
	if !ok {
		t.Fatalf("no read-deadline close was logged at production verbosity (V(0)); got:\n%s",
			strings.Join(logs.all(), "\n"))
	}
	for _, want := range []string{"closeKind", "readTimeout", readTimeout.String()} {
		if !strings.Contains(line, want) {
			t.Errorf("read-deadline line is missing %q, so the close is not attributable:\n%s", want, line)
		}
	}
}

// TestWriteDeadlineIsArmedForWritesThatBypassTheBuffer is the stale-deadline
// case: Flush is not the only path that touches the socket. A payload larger
// than the write buffer goes straight to the socket, and if the connection was
// idle for longer than writeTimeout first, a deadline left over from the last
// flush is already in the past - so a healthy peer would be closed on our own
// expired budget.
func TestWriteDeadlineIsArmedForWritesThatBypassTheBuffer(t *testing.T) {
	const writeTimeout = 100 * time.Millisecond

	proxySide, peer := net.Pipe()
	defer peer.Close()

	var received int64
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := peer.Read(buf)
			atomic.AddInt64(&received, int64(n))
			if err != nil {
				return
			}
		}
	}()

	w := NewWriter(proxySide, proto.ServerBound, writeTimeout, -1, logr.Discard())

	// One flush arms the deadline at now+writeTimeout; then stay idle past it,
	// as a connection with nothing to send would.
	if err := w.Flush(); err != nil {
		t.Fatalf("first flush: %v", err)
	}
	time.Sleep(writeTimeout + 50*time.Millisecond)

	// A payload bigger than the bufio.Writer buffer is written straight to the
	// socket while the peer is reading normally, so this must not fail.
	payload := make([]byte, 16*1024)
	n, err := w.Write(payload)
	if err != nil {
		t.Fatalf("writing %d bytes to a healthy, draining peer after %s of idle failed: %v "+
			"(the write deadline was armed by the last flush and had already expired)",
			len(payload), writeTimeout+50*time.Millisecond, err)
	}
	if n < len(payload) {
		// n includes the length prefix the encoder wrote in front of payload.
		t.Fatalf("wrote %d of %d bytes", n, len(payload))
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush after the direct write: %v", err)
	}

	waitFor := time.Now().Add(2 * time.Second)
	for atomic.LoadInt64(&received) < int64(len(payload)) && time.Now().Before(waitFor) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt64(&received); got < int64(len(payload)) {
		t.Fatalf("peer received %d of %d bytes", got, len(payload))
	}
	_ = proxySide.Close()
	<-readDone
}

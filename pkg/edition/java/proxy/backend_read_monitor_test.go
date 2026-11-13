package proxy

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/configutil"
)

// logCapture collects log lines from a funcr logger for assertions.
type logCapture struct {
	mu    sync.Mutex
	lines []string
}

func (c *logCapture) log(prefix, args string) {
	c.mu.Lock()
	c.lines = append(c.lines, strings.TrimSpace(prefix+" "+args))
	c.mu.Unlock()
}

func (c *logCapture) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

// waitFor returns the first captured line containing want, or fails the test.
func (c *logCapture) waitFor(t *testing.T, want string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		for _, line := range c.snapshot() {
			if strings.Contains(line, want) {
				return line
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no log line containing %q within %s; got:\n%s", want, timeout, strings.Join(c.snapshot(), "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (c *logCapture) countContaining(want string) int {
	n := 0
	for _, line := range c.snapshot() {
		if strings.Contains(line, want) {
			n++
		}
	}
	return n
}

// TestSilentBackendStallIsReported reproduces the measured plain-path stall (via
// disabled): a backend that accepts the TCP connection and never sends a single
// packet. The join used to sit there with nothing at all in Gate's log naming
// the backend, its address or the stage the connection had reached.
func TestSilentBackendStallIsReported(t *testing.T) {
	backendListener, backendContacted := startSilentBackend(t)

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	// The stall diagnostic is armed with the configured read timeout.
	cfg.ReadTimeout = configutil.Duration(300 * time.Millisecond)
	cfg.Servers = map[string]string{"lobby": backendListener.Addr().String()}
	cfg.Try = []string{"lobby"}

	logs := &logCapture{}
	proxy, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New: %v", err)
	}
	proxy.log = funcr.New(logs.log, funcr.Options{Verbosity: 1})
	if err := proxy.init(); err != nil {
		t.Fatalf("proxy init: %v", err)
	}

	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listener: %v", err)
	}
	defer proxyListener.Close()
	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go proxy.HandleConn(conn)
		}
	}()

	client, err := net.Dial("tcp", proxyListener.Addr().String())
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer client.Close()
	clientHost, clientPort, _ := net.SplitHostPort(proxyListener.Addr().String())
	clientPortNumber, _ := strconv.Atoi(clientPort)
	if err := writePlainHandshake(client, clientHost, clientPortNumber, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := writeServerLogin(client, "StallTester"); err != nil {
		t.Fatalf("client ServerLogin: %v", err)
	}

	// The backend must really have been contacted, otherwise this test proves
	// nothing about the backend path.
	select {
	case <-backendContacted:
	case <-time.After(5 * time.Second):
		t.Fatal("backend was never contacted by the proxy")
	}

	line := logs.waitFor(t, "has not sent a single packet", 10*time.Second)
	for _, want := range []string{
		`"backend"="lobby"`,
		`"address"="` + backendListener.Addr().String() + `"`,
		`"stage"="login"`,
		`"readTimeout"="300ms"`,
		`"waited"=`,
		`"hint"=`,
		`"msg"=`,
	} {
		if !strings.Contains(line, want) {
			t.Errorf("stall diagnostic missing %q:\n%s", want, line)
		}
	}
	if got := logs.countContaining("has not sent a single packet"); got != 1 {
		t.Errorf("stall diagnostic logged %d times, want exactly 1:\n%s", got, strings.Join(logs.snapshot(), "\n"))
	}
}

// TestStalledBackendDiagnostics locks the wording and the content of both
// teardown shapes: the backend that never spoke and the backend that went quiet
// for longer than the configured read timeout.
func TestStalledBackendDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		stall   backendStall
		message string
		extra   []string
	}{
		{
			name: "never sent a packet",
			stall: backendStall{
				backend:     "lobby",
				address:     "127.0.0.1:25566",
				stage:       backendStageLogin,
				bytesRead:   0,
				silentFor:   12 * time.Second,
				readTimeout: 30 * time.Second,
			},
			message: "never answered",
			extra:   []string{`"waited"=`, `"stage"="login"`},
		},
		{
			name: "went quiet past the read timeout",
			stall: backendStall{
				backend:     "survival",
				address:     "127.0.0.1:25567",
				stage:       backendStageConfiguration,
				bytesRead:   120,
				silentFor:   31 * time.Second,
				readTimeout: 30 * time.Second,
			},
			message: "stopped sending packets",
			extra:   []string{`"silentFor"=`, `"bytesRead"=120`, `"stage"="configuration"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := &logCapture{}
			logStalledBackend(funcr.New(logs.log, funcr.Options{Verbosity: 1}), tt.stall)

			line := logs.waitFor(t, tt.message, time.Second)
			wants := append([]string{
				`"backend"="` + tt.stall.backend + `"`,
				`"address"="` + tt.stall.address + `"`,
				`"readTimeout"="30s"`,
				`"hint"=`,
			}, tt.extra...)
			for _, want := range wants {
				if !strings.Contains(line, want) {
					t.Errorf("diagnostic missing %q:\n%s", want, line)
				}
			}
		})
	}
}

// TestBackendStallIsNotReportedWhenBackendSpoke is the control: a backend that
// already sent bytes and stopped recently must not be reported as stalled, or
// every backend restart would produce an error line.
func TestBackendStallIsNotReportedWhenBackendSpoke(t *testing.T) {
	logs := &logCapture{}
	log := funcr.New(logs.log, funcr.Options{Verbosity: 1})

	backendReader, backendWriter := net.Pipe()
	defer backendReader.Close()
	defer backendWriter.Close()

	monitor := &backendReadMonitor{Conn: backendReader}
	s := &serverConnection{
		server:             &registeredServer{info: NewServerInfo("lobby", mustParseAddr("127.0.0.1:25566"))},
		log:                log,
		backendRead:        monitor,
		backendStage:       backendStageLogin,
		backendConnectedAt: time.Now(),
	}
	read := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		_, _ = monitor.Read(buf)
		close(read)
	}()
	if _, err := backendWriter.Write([]byte("x")); err != nil {
		t.Fatalf("backend write: %v", err)
	}
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor never observed the backend byte")
	}
	s.backendConnectedAt = time.Now() // the backend just spoke

	// Neither the teardown report nor the while-stalled report may fire: the
	// backend answered, so its connection ending is not a stall.
	s.reportStalledBackend(log)
	s.reportSilentBackend(log)
	if got := logs.countContaining("backend server"); got != 0 {
		t.Errorf("answered backend must not be reported, got %d lines:\n%s", got, strings.Join(logs.snapshot(), "\n"))
	}
}

// TestStalledBackendShape locks the decision itself, including the shapes that
// must stay silent.
func TestStalledBackendShape(t *testing.T) {
	tests := []struct {
		name          string
		stall         backendStall
		neverAnswered bool
		wentQuiet     bool
	}{
		{
			name:          "never sent a byte",
			stall:         backendStall{bytesRead: 0, silentFor: time.Millisecond, readTimeout: 30 * time.Second},
			neverAnswered: true,
		},
		{
			name:      "answered then went quiet past the read timeout",
			stall:     backendStall{bytesRead: 120, silentFor: 31 * time.Second, readTimeout: 30 * time.Second},
			wentQuiet: true,
		},
		{
			name:  "answered recently",
			stall: backendStall{bytesRead: 120, silentFor: 2 * time.Second, readTimeout: 30 * time.Second},
		},
		{
			name:  "answered and the read timeout is disabled",
			stall: backendStall{bytesRead: 120, silentFor: time.Hour},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			neverAnswered, wentQuiet := stalledBackendShape(tt.stall)
			if neverAnswered != tt.neverAnswered || wentQuiet != tt.wentQuiet {
				t.Fatalf("stalledBackendShape(%+v) = (%v, %v), want (%v, %v)",
					tt.stall, neverAnswered, wentQuiet, tt.neverAnswered, tt.wentQuiet)
			}
		})
	}
}

// startSilentBackend starts a TCP listener that accepts connections and never
// sends a byte, and reports the first time a connection is accepted.
func startSilentBackend(t *testing.T) (net.Listener, <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	contacted := make(chan struct{}, 1)
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
			select {
			case contacted <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})
	return ln, contacted
}

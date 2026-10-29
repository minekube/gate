package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/configutil"
)

// The configured readTimeout/connectionTimeout are Go durations: "30s" and
// "30000" both decode to the duration written in the config (see
// configutil.Duration), and every other consumer in this module uses the value
// as it is. The connection layer must pass them on unchanged, because
// multiplying them by time.Millisecond once more turns the shipped 30s read
// timeout into ~347 days and the 5s connection timeout into ~57 days, i.e. into
// no timeout at all.
//
// The tests below assert the deadline that is really installed on a connection,
// observed at the socket, not the configured field: the configured field cannot
// show a second conversion.

type deadlineRecorder struct {
	net.Conn

	mu          sync.Mutex
	readDeltas  []time.Duration
	writeDeltas []time.Duration
}

// SetReadDeadline records how far in the future the deadline was set.
func (c *deadlineRecorder) SetReadDeadline(t time.Time) error {
	if !t.IsZero() {
		c.mu.Lock()
		c.readDeltas = append(c.readDeltas, time.Until(t))
		c.mu.Unlock()
	}
	return c.Conn.SetReadDeadline(t)
}

// SetWriteDeadline records how far in the future the deadline was set.
func (c *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	if !t.IsZero() {
		c.mu.Lock()
		c.writeDeltas = append(c.writeDeltas, time.Until(t))
		c.mu.Unlock()
	}
	return c.Conn.SetWriteDeadline(t)
}

// firstDelta returns the first recorded deadline of one direction, waiting for
// the connection layer to install it.
func (c *deadlineRecorder) firstDelta(t *testing.T, what string, deltas func() []time.Duration, timeout time.Duration) time.Duration {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		c.mu.Lock()
		recorded := deltas()
		c.mu.Unlock()
		if len(recorded) > 0 {
			return recorded[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s deadline was installed on the connection within %s", what, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c *deadlineRecorder) awaitReadDeadline(t *testing.T, timeout time.Duration) time.Duration {
	t.Helper()
	return c.firstDelta(t, "read", func() []time.Duration { return c.readDeltas }, timeout)
}

func (c *deadlineRecorder) awaitWriteDeadline(t *testing.T, timeout time.Duration) time.Duration {
	t.Helper()
	return c.firstDelta(t, "write", func() []time.Duration { return c.writeDeltas }, timeout)
}

// assertEffectiveTimeout asserts that the deadline the connection layer
// installed matches the configured timeout, and names the scaling factor
// otherwise.
func assertEffectiveTimeout(t *testing.T, what string, installed, configured time.Duration) {
	t.Helper()
	tolerance := configured/4 + 50*time.Millisecond
	if installed < configured-tolerance || installed > configured+tolerance {
		t.Errorf("%s: connection layer installed %s, configured %s (factor %.0fx)",
			what, installed.Round(time.Millisecond), configured,
			float64(installed)/float64(configured))
	}
}

// startProxy starts a proxy from cfg on a real listener and hands it every
// accepted client connection, recording the deadlines it installs on them.
func startProxy(t *testing.T, cfg config.Config, backendAddr string) (net.Listener, <-chan *deadlineRecorder) {
	t.Helper()
	recorders := make(chan *deadlineRecorder, 4)

	proxy, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New: %v", err)
	}
	proxy.log = logr.Discard()
	if err := proxy.init(); err != nil {
		t.Fatalf("proxy init: %v", err)
	}
	if backendAddr != "" {
		// A server whose dial the test controls is the only way to observe the
		// deadlines Gate installs on its own side of a backend connection.
		_, err = proxy.Register(&recordingServerInfo{
			name: "lobby",
			addr: mustParseAddr(backendAddr),
			dial: func(ctx context.Context, conn net.Conn) (net.Conn, error) {
				rec := &deadlineRecorder{Conn: conn}
				select {
				case recorders <- rec:
				default:
				}
				return rec, nil
			},
		})
		if err != nil {
			t.Fatalf("register backend server: %v", err)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			rec := &deadlineRecorder{Conn: conn}
			select {
			case recorders <- rec:
			default:
			}
			go proxy.HandleConn(rec)
		}
	}()
	return ln, recorders
}

// recordingServerInfo is a ServerInfo that also dials the backend itself, so a
// test can wrap the connection Gate gets in a deadlineRecorder.
type recordingServerInfo struct {
	name string
	addr net.Addr
	dial func(ctx context.Context, conn net.Conn) (net.Conn, error)
}

func (i *recordingServerInfo) Name() string   { return i.name }
func (i *recordingServerInfo) Addr() net.Addr { return i.addr }

func (i *recordingServerInfo) Dial(ctx context.Context, _ Player) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", i.addr.String())
	if err != nil {
		return nil, err
	}
	return i.dial(ctx, conn)
}

func waitRecorder(t *testing.T, recorders <-chan *deadlineRecorder) *deadlineRecorder {
	t.Helper()
	select {
	case rec := <-recorders:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatal("the proxy did not open/accept a connection")
		return nil
	}
}

// writeHandshakeWithIntent writes a plain (non-Forge) Handshake packet.
func writeHandshakeWithIntent(w io.Writer, host string, port, protocolVersion, intent int) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // Handshake
	pw.VarInt(protocolVersion)
	pw.String(host)
	_ = util.WriteUint16(&payload, uint16(port))
	pw.VarInt(intent)
	return writeFrame(w, payload.Bytes())
}

// joinClient dials the proxy and performs a plain login handshake.
func joinClient(t *testing.T, proxyAddr, username string) net.Conn {
	t.Helper()
	client, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	host, port, _ := net.SplitHostPort(proxyAddr)
	portNumber, _ := strconv.Atoi(port)
	if err := writeHandshakeWithIntent(client, host, portNumber, int(version.Minecraft_1_20.Protocol), 2); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := writeServerLogin(client, username); err != nil {
		t.Fatalf("client ServerLogin: %v", err)
	}
	return client
}

// TestClientConnectionDeadlinesMatchConfiguredTimeouts pins the read and write
// deadline installed on an accepted client connection: the read timeout is the
// per-read idle limit on the client, the connection timeout is the write
// timeout.
func TestClientConnectionDeadlinesMatchConfiguredTimeouts(t *testing.T) {
	const readTimeout = 1500 * time.Millisecond
	const connectionTimeout = 250 * time.Millisecond

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ReadTimeout = configutil.Duration(readTimeout)
	cfg.ConnectionTimeout = configutil.Duration(connectionTimeout)

	ln, recorders := startProxy(t, cfg, "")

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer client.Close()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	// Status intent plus a status request: the proxy answers on its own, so this
	// test needs no backend and the client stays in a state where no join
	// deadline can interfere.
	if err := writeHandshakeWithIntent(client, host, portNumber, int(version.Minecraft_1_20.Protocol), 1); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := writeFrame(client, []byte{0x00}); err != nil { // StatusRequest
		t.Fatalf("client StatusRequest: %v", err)
	}

	rec := waitRecorder(t, recorders)
	assertEffectiveTimeout(t, "client connection read deadline", rec.awaitReadDeadline(t, 2*time.Second), readTimeout)
	assertEffectiveTimeout(t, "client connection write deadline", rec.awaitWriteDeadline(t, 2*time.Second), connectionTimeout)
}

// TestBackendConnectionDeadlinesMatchConfiguredTimeouts pins the same deadlines
// on the connection Gate opens to a backend server: the read timeout bounds how
// long the backend may stay silent, the connection timeout is the write timeout
// of the handshake Gate sends it.
func TestBackendConnectionDeadlinesMatchConfiguredTimeouts(t *testing.T) {
	const readTimeout = 1500 * time.Millisecond
	const connectionTimeout = 250 * time.Millisecond

	backend, _ := startSilentBackend(t)

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ReadTimeout = configutil.Duration(readTimeout)
	cfg.ConnectionTimeout = configutil.Duration(connectionTimeout)
	cfg.Try = []string{"lobby"}

	ln, recorders := startProxy(t, cfg, backend.Addr().String())

	joinClient(t, ln.Addr().String(), "TimeoutTester")

	// Recorders arrive in connection order: the accepted client connection
	// first, then the backend connection Gate dialed.
	waitRecorder(t, recorders)
	rec := waitRecorder(t, recorders)
	assertEffectiveTimeout(t, "backend connection read deadline", rec.awaitReadDeadline(t, 2*time.Second), readTimeout)
	assertEffectiveTimeout(t, "backend connection write deadline", rec.awaitWriteDeadline(t, 2*time.Second), connectionTimeout)
}

// TestWithConnectionTimeoutUsesConfiguredDuration pins the conversion used for
// the join deadlines (the initial backend connection, server switches and the
// /server command), which are all derived from connectionTimeout.
func TestWithConnectionTimeoutUsesConfiguredDuration(t *testing.T) {
	const connectionTimeout = 250 * time.Millisecond

	cfg := config.DefaultConfig
	cfg.ConnectionTimeout = configutil.Duration(connectionTimeout)

	ctx, cancel := withConnectionTimeout(context.Background(), &cfg)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("withConnectionTimeout returned a context without a deadline")
	}
	assertEffectiveTimeout(t, "withConnectionTimeout deadline", time.Until(deadline), connectionTimeout)
}

// TestSilentBackendJoinIsBoundedByReadTimeout is the end-to-end case: a backend
// that accepts the TCP connection and never speaks must not hold a join open
// forever. With the shipped 30s read timeout that wait is bounded by it (scaled
// down here to keep the test fast). Both the client and the backend connection
// carry readTimeout, so this asserts that the join ends, not which end observed
// the deadline first - TestBackendConnectionDeadlinesMatchConfiguredTimeouts
// pins the backend deadline itself.
func TestSilentBackendJoinIsBoundedByReadTimeout(t *testing.T) {
	const readTimeout = 400 * time.Millisecond
	const observationWindow = 5 * time.Second

	backend, backendContacted := startSilentBackend(t)

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ReadTimeout = configutil.Duration(readTimeout)
	// Longer than the read timeout on purpose: the read timeout must be the
	// deadline that ends this join, not the connection timeout.
	cfg.ConnectionTimeout = configutil.Duration(10 * time.Second)
	cfg.Try = []string{"lobby"}

	ln, _ := startProxy(t, cfg, backend.Addr().String())

	client := joinClient(t, ln.Addr().String(), "StallTester")
	select {
	case <-backendContacted:
	case <-time.After(observationWindow):
		t.Fatal("backend was never contacted by the proxy")
	}

	// The proxy must end the join by closing the client connection (after
	// writing a disconnect packet, which is read here and ignored).
	if err := client.SetReadDeadline(time.Now().Add(observationWindow)); err != nil {
		t.Fatalf("client SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1024)
	var readErr error
	for {
		if _, readErr = client.Read(buf); readErr != nil {
			break
		}
	}
	var netErr net.Error
	if errors.As(readErr, &netErr) && netErr.Timeout() {
		t.Fatalf("the join to a silent backend was still open %s after the client went quiet "+
			"(configured readTimeout %s): the read timeout was not enforced", observationWindow, readTimeout)
	}
}

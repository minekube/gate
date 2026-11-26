package proxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component/codec/legacy"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// The configured timeouts are Go durations: "30s" and "30000" both decode to
// the duration written in the config (see configutil.Duration), and every other
// consumer in this module uses the value as it is. The connection layer must
// pass them on unchanged, because multiplying one by time.Millisecond once more
// turned the shipped 30s read timeout into ~347 days and the 5s connection
// timeout into ~57 days, i.e. into no timeout at all.
//
// writeTimeout is a separate budget from connectionTimeout: the per-flush write
// deadline of a client or backend connection is not the join budget. These
// tests assert the deadline that is really installed on a connection, observed
// at the socket, not the configured field: the field cannot show a second
// conversion.

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
// deadline installed on an accepted client connection: readTimeout bounds how
// long the client may stay silent, writeTimeout the per-flush write.
func TestClientConnectionDeadlinesMatchConfiguredTimeouts(t *testing.T) {
	const readTimeout = 1500 * time.Millisecond
	const writeTimeout = 250 * time.Millisecond

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ReadTimeout = configutil.Duration(readTimeout)
	cfg.WriteTimeout = configutil.Duration(writeTimeout)

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
	assertEffectiveTimeout(t, "client connection write deadline", rec.awaitWriteDeadline(t, 2*time.Second), writeTimeout)
}

// TestBackendConnectionDeadlinesMatchConfiguredTimeouts pins the same deadlines
// on the connection Gate opens to a backend server: readTimeout bounds how long
// the backend may stay silent, writeTimeout is the per-flush write deadline of
// the handshake Gate sends it.
func TestBackendConnectionDeadlinesMatchConfiguredTimeouts(t *testing.T) {
	const readTimeout = 1500 * time.Millisecond
	const writeTimeout = 250 * time.Millisecond

	backend, _ := startSilentBackend(t)

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ReadTimeout = configutil.Duration(readTimeout)
	cfg.WriteTimeout = configutil.Duration(writeTimeout)
	cfg.Try = []string{"lobby"}

	ln, recorders := startProxy(t, cfg, backend.Addr().String())

	joinClient(t, ln.Addr().String(), "TimeoutTester")

	// Recorders arrive in connection order: the accepted client connection
	// first, then the backend connection Gate dialed.
	waitRecorder(t, recorders)
	rec := waitRecorder(t, recorders)
	assertEffectiveTimeout(t, "backend connection read deadline", rec.awaitReadDeadline(t, 2*time.Second), readTimeout)
	assertEffectiveTimeout(t, "backend connection write deadline", rec.awaitWriteDeadline(t, 2*time.Second), writeTimeout)
}

// TestWithConnectionTimeoutUsesConfiguredDuration pins the conversion used for
// the join deadlines (the initial backend connection, server switches and the
// /server command), which are all derived from connectionTimeout - never from
// the write budget.
func TestWithConnectionTimeoutUsesConfiguredDuration(t *testing.T) {
	const connectionTimeout = 250 * time.Millisecond
	const writeTimeout = 10 * time.Millisecond

	cfg := config.DefaultConfig
	cfg.ConnectionTimeout = configutil.Duration(connectionTimeout)
	cfg.WriteTimeout = configutil.Duration(writeTimeout)

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
	// deadline that ends this join, not the join or the write budget.
	cfg.ConnectionTimeout = configutil.Duration(10 * time.Second)
	cfg.WriteTimeout = configutil.Duration(10 * time.Second)
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
	if isTimeoutErr(readErr) {
		t.Fatalf("the join to a silent backend was still open %s after the client went quiet "+
			"(configured readTimeout %s): the read timeout was not enforced", observationWindow, readTimeout)
	}
}

func isTimeoutErr(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}

// startDelayedLoginBackend starts a backend that accepts one connection, reads
// the proxy's handshake and login start, and only then - after delay - answers
// with LoginSuccess and JoinGame.
func startDelayedLoginBackend(t *testing.T, delay time.Duration, username string) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listener: %v", err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = ln.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
		if _, _, err := readPacket(conn); err != nil { // Handshake
			t.Logf("backend: read handshake: %v", err)
			return
		}
		if _, _, err := readPacket(conn); err != nil { // LoginStart
			t.Logf("backend: read login start: %v", err)
			return
		}
		// The backend is slow to answer the login without ever refusing to read:
		// Gate's writes to it drain fine, only the response is late.
		time.Sleep(delay)
		if err := writeServerLoginSuccess(conn, uuid.New(), username); err != nil {
			t.Logf("backend: LoginSuccess: %v", err)
			return
		}
		if err := writeJoinGame(conn); err != nil {
			t.Logf("backend: JoinGame: %v", err)
			return
		}
		// Stay open and keep reading so the proxy does not see a disconnect.
		buf := make([]byte, 4096)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
		}
	}()
	return ln
}

// TestBackendLoginSlowerThanWriteBudgetSucceeds is the regression the split
// exists for. The per-flush write budget must not become the join budget: a
// backend that needs longer than writeTimeout to answer the login, but less
// than connectionTimeout, still has to complete the join. Landing the
// un-scaling with a single shared field would fail this test (and, on the
// deployed edge, 26 of 144 successful Java joins in a 92-minute window took
// longer than 5s).
func TestBackendLoginSlowerThanWriteBudgetSucceeds(t *testing.T) {
	const writeBudget = 120 * time.Millisecond
	const loginDelay = 600 * time.Millisecond // slower than the write budget

	backend := startDelayedLoginBackend(t, loginDelay, "SlowLoginTester")

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ReadTimeout = configutil.Duration(30 * time.Second)
	cfg.ConnectionTimeout = configutil.Duration(5 * time.Second)
	cfg.WriteTimeout = configutil.Duration(writeBudget)
	cfg.Try = []string{"lobby"}

	ln, _ := startProxy(t, cfg, backend.Addr().String())
	start := time.Now()
	client := joinClient(t, ln.Addr().String(), "SlowLoginTester")
	_ = client.SetDeadline(time.Now().Add(10 * time.Second))

	// The client login completes on its own (the proxy sends LoginSuccess once
	// the player is authenticated); the backend join happens after that. The
	// backend's JoinGame relayed to the client is therefore the proof that the
	// join to a slow backend actually completed.
	for {
		id, _, err := readPacket(client)
		if err != nil {
			t.Fatalf("the client connection ended %s in, before the backend's %s login completed "+
				"(write budget %s, join budget %s): %v - the write budget became the join budget",
				time.Since(start).Round(time.Millisecond), loginDelay, writeBudget,
				time.Duration(cfg.ConnectionTimeout), err)
		}
		if id == 0x28 { // JoinGame, relayed from the backend
			if elapsed := time.Since(start); elapsed < loginDelay {
				t.Fatalf("the backend answered after %s, but the join completed in %s: the test never "+
					"exercised a login slower than the write budget (%s)", loginDelay, elapsed, writeBudget)
			}
			return
		}
	}
}

// TestBackendWriteDeadlineCloseShowsItsOwnMessage pins the player-facing text
// of a connection Gate itself closed on a configured deadline. It used to be
// the shared "Internal server connection error", which told the player nothing
// and made a drain/silence close look like a proxy bug.
func TestBackendWriteDeadlineCloseShowsItsOwnMessage(t *testing.T) {
	const writeTimeout = 80 * time.Millisecond

	peer, proxySide := net.Pipe() // the peer never reads, so a flush cannot drain
	defer peer.Close()
	defer proxySide.Close()

	backendConn, _ := netmc.NewMinecraftConn(
		context.Background(), proxySide, proto.ClientBound,
		30*time.Second, writeTimeout, -1, nil,
	)
	backendConn.SetActiveSessionHandler(state.Play, nopSessionHandler{})
	if err := backendConn.Flush(); err != nil { // seed the write deadline
		t.Fatalf("seeding the write deadline: %v", err)
	}
	if err := backendConn.Write(bytes.Repeat([]byte{0x01}, 64<<10)); err == nil {
		t.Fatal("a write to a peer that never reads must fail on the write deadline")
	}

	playerConn := &testMinecraftConn{}
	proxy := &Proxy{cfg: &config.Config{}}
	player := &connectedPlayer{
		MinecraftConn: playerConn,
		log:           logr.Discard(),
		profile:       &profile.GameProfile{ID: uuid.New(), Name: "DeadlineTester"},
	}
	player.sessionHandlerDeps = &sessionHandlerDeps{proxy: proxy}
	server := newRegisteredServer(NewServerInfo("lobby", mustParseAddr("127.0.0.1:25566")))
	serverConn := &serverConnection{
		server:     server,
		player:     player,
		log:        logr.Discard(),
		connection: backendConn,
	}
	handler := &backendPlaySessionHandler{serverConn: serverConn, log: logr.Discard()}

	handler.Disconnected()

	require.Len(t, playerConn.writtenPackets, 1, "the player must be told why the session ended")
	disconnect, ok := playerConn.writtenPackets[0].(*packet.Disconnect)
	require.Truef(t, ok, "expected a Disconnect packet, got %T", playerConn.writtenPackets[0])
	reason := disconnectReasonText(t, disconnect)
	require.NotContains(t, reason, "Internal server connection error",
		"a write-deadline close must not be reported as an internal proxy error")
	require.Contains(t, reason, "stopped accepting data",
		"a write-deadline close must tell the player the backend stopped draining what Gate sends")
}

// disconnectReasonText renders the reason of a Disconnect packet as the plain
// text a client shows.
func disconnectReasonText(t *testing.T, d *packet.Disconnect) string {
	t.Helper()
	require.NotNil(t, d.Reason, "Disconnect packet carries no reason")
	require.NotNil(t, d.Reason.Component, "Disconnect packet carries no component")
	b := new(strings.Builder)
	require.NoError(t, (&legacy.Legacy{}).Marshal(b, d.Reason.Component))
	return b.String()
}

// TestBackendReadDeadlineCloseShowsItsOwnMessage is the silence half of the
// same rule: a backend that stops sending for readTimeout is closed by Gate's
// read deadline, and the player is told the server stopped responding rather
// than being shown an internal proxy error.
func TestBackendReadDeadlineCloseShowsItsOwnMessage(t *testing.T) {
	const readTimeout = 80 * time.Millisecond

	peer, proxySide := net.Pipe() // the peer never sends a byte
	defer peer.Close()
	defer proxySide.Close()

	backendConn, readLoop := netmc.NewMinecraftConn(
		context.Background(), proxySide, proto.ClientBound,
		readTimeout, 30*time.Second, -1, nil,
	)
	backendConn.SetActiveSessionHandler(state.Play, nopSessionHandler{})
	go readLoop()

	deadline := time.Now().Add(2 * time.Second)
	for !netmc.Closed(backendConn) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.True(t, netmc.Closed(backendConn), "the read deadline did not close the connection")

	playerConn := &testMinecraftConn{}
	proxy := &Proxy{cfg: &config.Config{}}
	player := &connectedPlayer{
		MinecraftConn: playerConn,
		log:           logr.Discard(),
		profile:       &profile.GameProfile{ID: uuid.New(), Name: "SilenceTester"},
	}
	player.sessionHandlerDeps = &sessionHandlerDeps{proxy: proxy}
	server := newRegisteredServer(NewServerInfo("lobby", mustParseAddr("127.0.0.1:25566")))
	serverConn := &serverConnection{
		server:     server,
		player:     player,
		log:        logr.Discard(),
		connection: backendConn,
	}
	handler := &backendPlaySessionHandler{serverConn: serverConn, log: logr.Discard()}

	handler.Disconnected()

	require.Len(t, playerConn.writtenPackets, 1, "the player must be told why the session ended")
	disconnect, ok := playerConn.writtenPackets[0].(*packet.Disconnect)
	require.Truef(t, ok, "expected a Disconnect packet, got %T", playerConn.writtenPackets[0])
	reason := disconnectReasonText(t, disconnect)
	require.NotContains(t, reason, "Internal server connection error",
		"a read-deadline close must not be reported as an internal proxy error")
	require.Contains(t, reason, "stopped responding",
		"a read-deadline close must tell the player the server stopped responding")
}

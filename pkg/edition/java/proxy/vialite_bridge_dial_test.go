package proxy

import (
	"context"
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

// dynamicDialServer is a Connect-shaped dynamic backend: a ServerInfo that also
// implements ServerDialer, which is how a tunnel-backed server (the only kind
// that gets a translation bridge) is registered. Gate never dials the address
// it reports for such a server - it dials the bridge the runner registered with
// the hop, and the bridge calls this Dial.
type dynamicDialServer struct {
	name string
	addr net.Addr
	dial func(context.Context, Player) (net.Conn, error)
}

func (d *dynamicDialServer) Name() string   { return d.name }
func (d *dynamicDialServer) Addr() net.Addr { return d.addr }
func (d *dynamicDialServer) Dial(ctx context.Context, p Player) (net.Conn, error) {
	return d.dial(ctx, p)
}

// TestDynamicBackendBridgeDialIsBoundedAndNamed covers the one join phase that
// this card could only hypothesise about: a join to a *dynamic* backend
// (ServerDialer + translation bridge, i.e. the Connect shape) whose dial never
// answers.
//
// The bridge request context is deliberately detached from the join
// (context.WithoutCancel, so a claimed bridge survives the request context
// ending), which also removes the join's deadline from it. A dynamic backend
// that never answered therefore left the join with no bound at all - and with
// nothing naming the backend, the bridge address or the stage, because the
// bridge discarded the dial error.
func TestDynamicBackendBridgeDialIsBoundedAndNamed(t *testing.T) {
	staticBackend, _ := startSilentBackend(t)

	dialStarted := make(chan struct{})
	var once sync.Once
	dynamic := &dynamicDialServer{
		name: "dyn",
		addr: mustParseAddr("127.0.0.1:26500"),
		dial: func(ctx context.Context, _ Player) (net.Conn, error) {
			once.Do(func() { close(dialStarted) })
			<-ctx.Done() // backend never answers: stalled Connect tunnel dial
			return nil, ctx.Err()
		},
	}

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	// The documented budget for reaching a backend. The bridge dial must honour
	// it in the units it is written in: the join's own deadline cannot bound
	// this dial (see above).
	cfg.ConnectionTimeout = configutil.Duration(300 * time.Millisecond)
	cfg.Servers = map[string]string{"lobby": staticBackend.Addr().String()}
	cfg.Try = []string{"dyn"}
	cfg.Via.Enabled = true

	fake := &fakeVialiteServer{useBackendAddress: true}
	_, logs, client, bridgeAddr := startJoinWithDynamicBackend(t, &cfg, fake, dynamic)

	// The dynamic path must really be in play: Gate dialled the bridge the
	// runner registered, and the bridge claimed the join's request.
	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatalf("the dynamic backend was never dialled through the bridge; log:\n%s",
			strings.Join(logs.snapshot(), "\n"))
	}

	// 1. The stalled dial is abandoned within the configured budget and named.
	line := logs.waitFor(t, "behind the vialite bridge", 5*time.Second)
	for _, want := range []string{
		`"backend"=` + `"dyn"`,
		`"address"=` + `"` + bridgeAddr + `"`,
		`"stage"=` + `"` + viaBridgeDialStage + `"`,
		"deadline exceeded",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("bridge dial diagnostic missing %q:\n%s", want, line)
		}
	}

	// 2. The join itself ends bounded and carries the named cause, instead of
	// sitting on a backend whose address is the only thing the operator sees.
	joinLine := logs.waitFor(t, "unable to connect to server", 5*time.Second)
	for _, want := range []string{"dyn", bridgeAddr, viaBridgeDialStage} {
		if !strings.Contains(joinLine, want) {
			t.Errorf("join failure message missing %q:\n%s", want, joinLine)
		}
	}

	// 3. And the client is released: the connection ends rather than hanging.
	expectClientTeardown(t, client, 5*time.Second)
}

// TestDynamicBackendBridgeDialFailureIsNotReportedWhenTheBackendAnswers is the
// control: the same dynamic shape, but the dial answers immediately. No bridge
// dial failure may be reported, and the join must still reach the backend - the
// probe here is a backend that accepts and never speaks, which the existing
// post-dial diagnostic names.
func TestDynamicBackendBridgeDialFailureIsNotReportedWhenTheBackendAnswers(t *testing.T) {
	staticBackend, _ := startSilentBackend(t)
	silentBackend, _ := startSilentBackend(t)

	dynamic := &dynamicDialServer{
		name: "dyn",
		addr: mustParseAddr("127.0.0.1:26500"),
		dial: func(context.Context, Player) (net.Conn, error) {
			return net.Dial("tcp", silentBackend.Addr().String())
		},
	}

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.ConnectionTimeout = configutil.Duration(300 * time.Millisecond)
	cfg.ReadTimeout = configutil.Duration(300 * time.Millisecond)
	cfg.Servers = map[string]string{"lobby": staticBackend.Addr().String()}
	cfg.Try = []string{"dyn"}
	cfg.Via.Enabled = true

	fake := &fakeVialiteServer{useBackendAddress: true}
	_, logs, _, _ := startJoinWithDynamicBackend(t, &cfg, fake, dynamic)

	line := logs.waitFor(t, "has not sent a single packet", 10*time.Second)
	if !strings.Contains(line, `"backend"="dyn"`) {
		t.Errorf("silent backend diagnostic must name the dynamic backend:\n%s", line)
	}
	if got := logs.countContaining("behind the vialite bridge"); got != 0 {
		t.Errorf("a backend that answered must not be reported as a bridge dial failure, got %d lines:\n%s",
			got, strings.Join(logs.snapshot(), "\n"))
	}
}

// TestBridgeDialFailureErrorNamesBackendAddressAndStage locks the shape of the
// error the join reports.
func TestBridgeDialFailureErrorNamesBackendAddressAndStage(t *testing.T) {
	cause := context.DeadlineExceeded
	err := bridgeDialFailureError("dyn", "127.0.0.1:41234", cause)
	msg := err.Error()
	for _, want := range []string{
		"dyn", "127.0.0.1:41234", viaBridgeDialStage, cause.Error(), "bridge",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("bridge dial failure %q missing %q", msg, want)
		}
	}
}

// startJoinWithDynamicBackend boots a proxy with via enabled, registers the
// dynamic backend and drives one raw client join to it. It returns the proxy,
// the captured log, the (still open) client connection and the bridge address
// the hop dials.
func startJoinWithDynamicBackend(
	t *testing.T,
	cfg *config.Config,
	fake *fakeVialiteServer,
	dynamic *dynamicDialServer,
) (*Proxy, *logCapture, net.Conn, string) {
	t.Helper()

	logs := &logCapture{}
	proxy, err := New(Options{Config: cfg})
	if err != nil {
		t.Fatalf("proxy New: %v", err)
	}
	proxy.log = funcr.New(logs.log, funcr.Options{Verbosity: 1})
	proxy.via = &viaManagedRunner{
		cfg:            cfg,
		server:         fake,
		activeBackends: map[string]struct{}{"lobby": {}},
	}
	if err := proxy.init(); err != nil {
		t.Fatalf("proxy init: %v", err)
	}

	if _, err := proxy.Register(dynamic); err != nil {
		t.Fatalf("register dynamic backend: %v", err)
	}
	if _, ok := proxy.Server("dyn").ServerInfo().(ServerDialer); !ok {
		t.Fatalf("dynamic server was not wrapped for the hop: %T", proxy.Server("dyn").ServerInfo())
	}
	bridgeAddr := fake.backends["dyn"]
	if bridgeAddr == "" {
		t.Fatal("the runner did not register a bridge address for the dynamic backend")
	}
	if bridgeAddr == dynamic.addr.String() {
		t.Fatalf("dynamic backend must be dialled through the bridge, got its own address %s", bridgeAddr)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go proxy.HandleConn(conn)
		}
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	if err := writePlainHandshake(client, host, portNumber, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	if err := writeServerLogin(client, "BridgeDialTester"); err != nil {
		t.Fatalf("client ServerLogin: %v", err)
	}
	return proxy, logs, client, bridgeAddr
}

// expectClientTeardown asserts the client connection ends (Gate disconnects the
// player) within the given budget instead of hanging.
func expectClientTeardown(t *testing.T, client net.Conn, within time.Duration) {
	t.Helper()
	if err := client.SetReadDeadline(time.Now().Add(within)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 4096)
	for {
		if _, err := client.Read(buf); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatalf("client connection still open after %s, want teardown", within)
			}
			return
		}
	}
}

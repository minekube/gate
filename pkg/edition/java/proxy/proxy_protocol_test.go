package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/netutil"
)

// proxyProtocolTestMotd is the status response text a successful ping returns,
// proving the client's byte stream reached the proxy intact.
const proxyProtocolTestMotd = "gate-proxy-protocol-test"

// spoofedClientAddr is the address a forged PROXY protocol header claims to
// come from. It must never be attributed to a connection that did not arrive
// through a trusted upstream, or an attacker could evade an IP ban and have an
// innocent third party banned in their place.
var spoofedClientAddr = &net.TCPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 43210}

// TestProxyProtocolForgedHeaderFromUntrustedSourceIsNotHonored is the
// regression test for the PROXY protocol spoofing hole: accepted connections
// used to be wrapped with proxyproto.NewConn(conn) without any policy, which
// defaults to proxyproto.USE, so every peer able to reach the bind address
// could assert an arbitrary client IP.
func TestProxyProtocolForgedHeaderFromUntrustedSourceIsNotHonored(t *testing.T) {
	// Loopback (where the test client connects from) is deliberately not trusted.
	gate := startProxyProtocolProxy(t, func(c *config.Config) {
		c.ProxyProtocol = true
		c.ProxyProtocolTrustedProxies = []string{"192.0.2.0/24"}
	})

	conn := gate.dial(t)
	writeProxyHeader(t, conn)

	observed := gate.observedRemoteAddr(t)
	require.NotEqual(t, "1.2.3.4", netutil.Host(observed),
		"forged PROXY header from an untrusted peer must not set the client address")
	require.Equal(t, "127.0.0.1", netutil.Host(observed),
		"the real transport peer address must be used instead")
}

// TestProxyProtocolHeaderFromTrustedUpstreamIsHonored is the other half of the
// contract: a PROXY header from a trusted upstream still carries the real
// client address, which is what the feature exists for.
func TestProxyProtocolHeaderFromTrustedUpstreamIsHonored(t *testing.T) {
	gate := startProxyProtocolProxy(t, func(c *config.Config) {
		c.ProxyProtocol = true
		c.ProxyProtocolTrustedProxies = []string{"127.0.0.0/8"}
	})

	conn := gate.dial(t)
	writeProxyHeader(t, conn)
	require.NoError(t, writeStatusHandshake(conn, gate.bind, "localhost", 765))

	require.Equal(t, "1.2.3.4", netutil.Host(gate.observedRemoteAddr(t)),
		"PROXY header from a trusted upstream must set the client address")
}

func TestProxyProtocolRawWireTelemetryCountsV1BeforeRemoteAddr(t *testing.T) {
	collector := newLockedTelemetryEvents()
	gate := startProxyProtocolProxyWithObserver(t, func(c *config.Config) {
		c.ProxyProtocol = true
		c.ProxyProtocolTrustedProxies = []string{"127.0.0.0/8"}
	}, collector)
	conn := gate.dial(t)
	const header = "PROXY TCP4 1.2.3.4 10.0.0.1 43210 25565\r\n"
	_, err := conn.Write([]byte(header))
	require.NoError(t, err)
	require.NoError(t, writeStatusHandshake(conn, gate.bind, "localhost", 765))
	_, err = readStatusResponse(conn)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	collector.waitTerminals(t, 1)
	if got := collector.readBytes(); got < int64(len(header)) {
		t.Fatalf("raw telemetry read bytes = %d, want at least PROXY v1 header %d", got, len(header))
	}
}

func TestProxyProtocolRateLimitedReturnCountsV2Header(t *testing.T) {
	collector := newLockedTelemetryEvents()
	gate := startProxyProtocolProxyWithObserver(t, func(c *config.Config) {
		c.ProxyProtocol = true
		c.ProxyProtocolTrustedProxies = []string{"127.0.0.0/8"}
		c.Quota.Connections.Enabled = true
		c.Quota.Connections.OPS = 0.01
		c.Quota.Connections.Burst = 1
	}, collector)
	first := gate.dial(t)
	writeProxyHeader(t, first) // consumes the only token for spoofedClientAddr
	require.NoError(t, first.Close())
	collector.waitTerminals(t, 1)
	before := collector.readBytes()
	second := gate.dial(t)
	writeProxyHeader(t, second)
	require.NoError(t, second.Close())
	collector.waitTerminals(t, 2)
	if got := collector.readBytes() - before; got < 28 { // fixed v2 header, excluding address payload
		t.Fatalf("rate-limited raw telemetry bytes = %d, want PROXY v2 header", got)
	}
}

// TestProxyProtocolUntrustedClientWithoutHeaderIsUnaffected makes sure the
// hardening does not reject regular Minecraft clients, which never send a PROXY
// header and connect from untrusted addresses.
func TestProxyProtocolUntrustedClientWithoutHeaderIsUnaffected(t *testing.T) {
	gate := startProxyProtocolProxy(t, func(c *config.Config) {
		c.ProxyProtocol = true
		c.ProxyProtocolTrustedProxies = []string{"192.0.2.0/24"}
	})

	conn := gate.dial(t)
	require.NoError(t, writeStatusHandshake(conn, gate.bind, "localhost", 765))

	status, err := readStatusResponse(conn)
	require.NoError(t, err, "a client that sends no PROXY header must pass through")
	require.Contains(t, status, proxyProtocolTestMotd)
	require.Equal(t, "127.0.0.1", netutil.Host(gate.observedRemoteAddr(t)))
}

// TestProxyProtocolDisabledIsPassthrough pins that nothing changes while the
// feature is off: no header sniffing, no consumed bytes, real peer address.
func TestProxyProtocolDisabledIsPassthrough(t *testing.T) {
	gate := startProxyProtocolProxy(t, func(c *config.Config) {
		c.ProxyProtocol = false
		c.ProxyProtocolTrustedProxies = []string{"127.0.0.0/8"} // trusted, but unused
	})

	plain := gate.dial(t)
	require.NoError(t, writeStatusHandshake(plain, gate.bind, "localhost", 765))

	status, err := readStatusResponse(plain)
	require.NoError(t, err)
	require.Contains(t, status, proxyProtocolTestMotd, "a normal handshake must be forwarded untouched")
	require.Equal(t, "127.0.0.1", netutil.Host(gate.observedRemoteAddr(t)))

	// Even a peer that would be trusted cannot rewrite its address while the
	// feature is disabled: the header is never looked at.
	spoofer := gate.dial(t)
	writeProxyHeader(t, spoofer)
	require.Equal(t, "127.0.0.1", netutil.Host(gate.observedRemoteAddr(t)))
}

// TestProxyProtocolReadHeaderTimeoutBoundsStalledSniff covers the second half
// of the defect: proxyproto.NewConn applies no read deadline of its own (only
// proxyproto.Listener does), so a peer that connects and sends nothing used to
// park a goroutine and file descriptor indefinitely.
func TestProxyProtocolReadHeaderTimeoutBoundsStalledSniff(t *testing.T) {
	pp, err := newProxyProtocol(&config.Config{})
	require.NoError(t, err)

	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() { _ = server.Close() })

	// The client never sends anything, so the sniff can only end on its deadline.
	conn := pp.wrapConnTimeout(server, 100*time.Millisecond)
	sniffed := make(chan net.Addr, 1)
	go func() { sniffed <- conn.RemoteAddr() }()

	select {
	case addr := <-sniffed:
		require.Equal(t, server.RemoteAddr().String(), addr.String())
	case <-time.After(5 * time.Second):
		t.Fatal("PROXY protocol header sniff did not honor the read header timeout")
	}
}

// TestProxyProtocolDefaultTrustedProxies pins the default trust set: private
// and loopback networks where a PROXY protocol sender is normally deployed,
// never a public address.
func TestProxyProtocolDefaultTrustedProxies(t *testing.T) {
	pp, err := newProxyProtocol(&config.Config{ProxyProtocol: true})
	require.NoError(t, err)

	trusted := map[string]bool{
		"127.0.0.1:25565":        true,
		"10.0.0.5:25565":         true,
		"172.16.9.9:25565":       true,
		"192.168.1.10:25565":     true,
		"[::1]:25565":            true,
		"[fdaa:0:1::3]:25565":    true, // Fly.io 6PN
		"1.2.3.4:25565":          false,
		"203.0.113.9:25565":      false,
		"[2001:db8::1234]:25565": false,
		"8.8.8.8:53":             false,
	}
	for addr, want := range trusted {
		require.Equalf(t, want, pp.trustedNetworks().Contains(netutil.NewAddr(addr, "tcp")),
			"default trust of %s", addr)
	}

	_, err = newProxyProtocol(&config.Config{ProxyProtocolTrustedProxies: []string{"not-an-ip"}})
	require.Error(t, err, "malformed trusted upstreams must not be silently ignored")

	// A nil wrapper fails closed rather than honoring forged headers.
	var unset *proxyProtocol
	require.False(t, unset.trustedNetworks().Contains(netutil.NewAddr("127.0.0.1:25565", "tcp")))
}

// proxyProtocolProxy is a running Gate instance that records the remote address
// every accepted connection resolved to.
type proxyProtocolProxy struct {
	bind    string
	remotes chan net.Addr
}

func startProxyProtocolProxy(t *testing.T, configure func(*config.Config)) *proxyProtocolProxy {
	return startProxyProtocolProxyWithObserver(t, configure, nil)
}

func startProxyProtocolProxyWithObserver(t *testing.T, configure func(*config.Config), observer connectiontelemetry.Observer) *proxyProtocolProxy {
	t.Helper()

	motd, err := componentutil.ParseComponent(version.MaximumVersion.Protocol, proxyProtocolTestMotd)
	require.NoError(t, err)

	cfg := config.DefaultConfig
	cfg.Bind = reserveTCPAddress(t)
	cfg.Status.Motd = &configutil.Component{Value: motd}
	cfg.OnlineMode = false
	cfg.Quota.Connections.Enabled = false
	cfg.Quota.Logins.Enabled = false
	cfg.PacketLimiter.PacketsPerSecond = -1
	cfg.PacketLimiter.BytesPerSecond = -1
	configure(&cfg)

	gate := &proxyProtocolProxy{bind: cfg.Bind, remotes: make(chan net.Addr, 8)}

	events := event.New(event.WithRecoverPanic(false))
	ready := make(chan struct{})
	t.Cleanup(event.Subscribe(events, 0, func(*ReadyEvent) {
		select {
		case <-ready:
		default:
			close(ready)
		}
	}))
	t.Cleanup(event.Subscribe(events, 0, func(e *ConnectionEvent) {
		select {
		case gate.remotes <- e.Connection().RemoteAddr():
		default:
		}
	}))

	p, err := New(Options{Config: &cfg, EventMgr: events})
	require.NoError(t, err)
	if observer != nil {
		p.connectionObservations = observer
	}

	ctx, cancel := context.WithCancel(context.Background())
	startResult := make(chan error, 1)
	go func() { startResult <- p.Start(ctx) }()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("Gate did not become ready")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-startResult:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Error("Gate did not stop")
		}
	})

	return gate
}

type lockedTelemetryEvents struct {
	mu     sync.Mutex
	events []connectiontelemetry.Event
}

func newLockedTelemetryEvents() *lockedTelemetryEvents { return new(lockedTelemetryEvents) }
func (c *lockedTelemetryEvents) Observe(_ context.Context, event connectiontelemetry.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}
func (c *lockedTelemetryEvents) readBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	var total int64
	for _, event := range c.events {
		total += event.BytesRead
	}
	return total
}
func (c *lockedTelemetryEvents) waitTerminals(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		count := 0
		for _, event := range c.events {
			if event.Terminal {
				count++
			}
		}
		c.mu.Unlock()
		if count >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d terminal telemetry events", want)
}

func (g *proxyProtocolProxy) dial(t *testing.T) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", g.bind, 5*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	return conn
}

// observedRemoteAddr returns the remote address Gate resolved for the next
// accepted connection.
func (g *proxyProtocolProxy) observedRemoteAddr(t *testing.T) net.Addr {
	t.Helper()
	select {
	case addr := <-g.remotes:
		return addr
	case <-time.After(10 * time.Second):
		t.Fatal("Gate did not report a connection")
		return nil
	}
}

// writeStatusHandshake sends the handshake and status request a client uses to
// ping a server, so the connection carries a valid Minecraft stream after any
// PROXY protocol header.
func writeStatusHandshake(w net.Conn, gateAddr, host string, protocol int) error {
	var handshake bytes.Buffer
	pw := util.PanicWriter(&handshake)
	pw.VarInt(0)
	pw.VarInt(protocol)
	pw.String(host)
	if err := util.WriteUint16(&handshake, netutil.PortStr(gateAddr)); err != nil {
		return fmt.Errorf("encode handshake port: %w", err)
	}
	pw.VarInt(1)
	if err := writeStatusFrame(w, handshake.Bytes()); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}
	return writeStatusFrame(w, []byte{0})
}

// readStatusResponse reads the status response a ping returns.
func readStatusResponse(r net.Conn) (string, error) {
	id, payload, err := readStatusFrame(r)
	if err != nil {
		return "", fmt.Errorf("read status response: %w", err)
	}
	if id != 0 {
		return "", fmt.Errorf("status response packet id=%d, want 0", id)
	}
	return util.ReadString(bytes.NewReader(payload))
}

// writeProxyHeader prepends a PROXY protocol v2 header claiming to come from
// spoofedClientAddr, exactly like an upstream proxy would.
func writeProxyHeader(t *testing.T, w net.Conn) {
	t.Helper()
	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        spoofedClientAddr,
		DestinationAddr:   &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 25565},
	}
	_, err := header.WriteTo(w)
	require.NoError(t, err)
}

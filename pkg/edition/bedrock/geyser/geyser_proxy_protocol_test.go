package geyser

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/pires/go-proxyproto"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	bconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/netutil"
)

// javaHandshake is the first thing a Geyser sends on the Geyser listener: a
// Minecraft handshake frame. Its first byte is the frame length varint, which is
// what the Java reader chokes on when the PROXY header it required is missing.
var javaHandshake = []byte{0x10, 0x00, 0x88, 0x02, 0x0c, 0x31, 0x39, 0x32, 0x2e, 0x31, 0x36, 0x38, 0x2e, 0x31, 0x2e, 0x31, 0x63, 0xdd}

// newGeyserListenerIntegration builds the Geyser listener side of a Bedrock
// integration with Gate's default PROXY protocol trusted-upstream policy
// (loopback, RFC 1918, link-local and ULA are trusted; the public internet is
// not).
func newGeyserListenerIntegration(t *testing.T) *Integration {
	t.Helper()
	cfg := jconfig.DefaultConfig
	javaProxy, err := proxy.New(proxy.Options{Config: &cfg, EventMgr: event.Nop})
	require.NoError(t, err)
	log, _ := capturingLogger()
	return &Integration{
		ctx:    context.Background(),
		log:    log,
		proxy:  javaProxy,
		config: &bconfig.Config{GeyserListenAddr: "localhost:25567"},
	}
}

// addrConn presents a fabricated remote address over an in-memory pipe, so a
// test can place the peer inside or outside proxyProtocolTrustedProxies without
// binding a socket.
type addrConn struct {
	net.Conn
	remote net.Addr
}

func (c addrConn) RemoteAddr() net.Addr { return c.remote }

// pipeToPeer returns the listener-side connection of a net.Pipe pair whose peer
// address is remote, plus a function that writes from the upstream and returns
// the write error.
func pipeToPeer(t *testing.T, remote string) (net.Conn, func([]byte) error) {
	t.Helper()
	listenerSide, upstreamSide := net.Pipe()
	t.Cleanup(func() {
		_ = listenerSide.Close()
		_ = upstreamSide.Close()
	})
	write := func(p []byte) error {
		_, err := upstreamSide.Write(p)
		return err
	}
	return addrConn{Conn: listenerSide, remote: netutil.NewAddr(remote, "tcp")}, write
}

// TestGeyserListenerToleratesHeaderlessUpstream is the regression for the
// managed GeyserLite outage: Geyser connected to the Geyser listener without a
// PROXY header (its HAProxy toggle was rendered at a key Geyser 2.x drops) and
// every join died on the first read with
// "proxyproto: proxy protocol signature not present". A headerless upstream must
// now reach the Java handshake reader with its bytes intact.
func TestGeyserListenerToleratesHeaderlessUpstream(t *testing.T) {
	t.Parallel()
	integration := newGeyserListenerIntegration(t)

	listenerSide, write := pipeToPeer(t, "127.0.0.1:51000")
	geyserConn := integration.wrapAcceptedConn(listenerSide)

	go func() { _ = write(javaHandshake) }()

	got := make([]byte, len(javaHandshake))
	_, err := io.ReadFull(geyserConn, got)
	require.NoError(t, err, "a headerless Geyser peer must not fail its first read")
	require.Equal(t, javaHandshake, got, "the handshake bytes must survive the wrapper untouched")
	require.Equal(t, "127.0.0.1:51000", geyserConn.RemoteAddr().String(),
		"without a header the peer's own address stays the remote address")
}

// TestGeyserListenerHonorsHeaderFromTrustedUpstream keeps the real-client-IP
// half: when Geyser does send a PROXY v2 header from a trusted (by default,
// loopback) address, the header's source address becomes the session's remote
// address.
func TestGeyserListenerHonorsHeaderFromTrustedUpstream(t *testing.T) {
	t.Parallel()
	integration := newGeyserListenerIntegration(t)

	listenerSide, write := pipeToPeer(t, "127.0.0.1:51001")
	geyserConn := integration.wrapAcceptedConn(listenerSide)

	clientAddr := &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 41234}
	header := proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        clientAddr,
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 25567},
	}
	var prefix bytes.Buffer
	_, err := header.WriteTo(&prefix)
	require.NoError(t, err)

	go func() { _ = write(append(prefix.Bytes(), javaHandshake...)) }()

	got := make([]byte, len(javaHandshake))
	_, err = io.ReadFull(geyserConn, got)
	require.NoError(t, err)
	require.Equal(t, javaHandshake, got, "the header must be stripped, not passed to the Java reader")
	require.Equal(t, clientAddr.String(), geyserConn.RemoteAddr().String(),
		"a header from a trusted upstream must supply the real Bedrock client address")
}

// TestGeyserListenerRejectsHeaderFromUntrustedUpstream is the trust boundary:
// a peer outside proxyProtocolTrustedProxies may not assert a client address,
// and the refusal is reported once, at INFO, naming the cause instead of
// surfacing as a bare varint error.
func TestGeyserListenerRejectsHeaderFromUntrustedUpstream(t *testing.T) {
	t.Parallel()

	log, capture := capturingLogger()
	cfg := jconfig.DefaultConfig
	javaProxy, err := proxy.New(proxy.Options{Config: &cfg, EventMgr: event.Nop})
	require.NoError(t, err)
	integration := &Integration{
		ctx:    context.Background(),
		log:    log,
		proxy:  javaProxy,
		config: &bconfig.Config{GeyserListenAddr: "localhost:25567"},
	}

	listenerSide, write := pipeToPeer(t, "198.51.100.9:51002")
	geyserConn := integration.wrapAcceptedConn(listenerSide)

	header := proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("203.0.113.7"), Port: 41234},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("198.51.100.9"), Port: 25567},
	}
	var prefix bytes.Buffer
	_, err = header.WriteTo(&prefix)
	require.NoError(t, err)

	go func() { _ = write(append(prefix.Bytes(), javaHandshake...)) }()

	buf := make([]byte, len(javaHandshake))
	_, err = geyserConn.Read(buf)
	require.ErrorIs(t, err, proxyproto.ErrSuperfluousProxyHeader,
		"an untrusted upstream must not be able to assert a client address")

	// The rejection is reported once, not per failed read.
	_, err = geyserConn.Read(buf)
	require.ErrorIs(t, err, proxyproto.ErrSuperfluousProxyHeader)

	logged := capture()
	require.Equal(t, 1, strings.Count(logged, "rejected PROXY protocol header from an untrusted upstream"),
		"the cause must be logged exactly once per connection:\n%s", logged)
	require.Contains(t, logged, "proxyProtocolTrustedProxies",
		"the log line must name the cause so an operator can fix it")
	require.Contains(t, logged, "listenAddr", "the log line must carry the listener it applies to")
	require.Contains(t, logged, "localhost:25567")
}

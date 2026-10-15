package lite

import (
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/lite/voicechat"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/configutil"
)

// voiceSocket is a fake Simple Voice Chat backend UDP endpoint.
type voiceSocket struct {
	conn *net.UDPConn
	got  chan struct{}
}

func listenVoiceSocket(t *testing.T, addr *net.UDPAddr) *voiceSocket {
	t.Helper()
	c, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	v := &voiceSocket{conn: c, got: make(chan struct{}, 256)}
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, _, err := c.ReadFromUDP(buf); err != nil {
				return
			}
			v.got <- struct{}{}
		}
	}()
	return v
}

// voiceSocketPair binds the same UDP port on 127.0.0.1 and [::1]. Both
// loopback families exist on every dev host, which lets two backends differ
// only by IP, exactly like two hosts sharing the default voice port.
func voiceSocketPair(t *testing.T) (v4, v6 *voiceSocket, port int) {
	t.Helper()
	for range 20 {
		c4, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		require.NoError(t, err)
		p := c4.LocalAddr().(*net.UDPAddr).Port
		c6, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv6loopback, Port: p})
		if err != nil {
			_ = c4.Close()
			continue
		}
		_ = c4.Close()
		_ = c6.Close()
		return listenVoiceSocket(t, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: p}),
			listenVoiceSocket(t, &net.UDPAddr{IP: net.IPv6loopback, Port: p}), p
	}
	t.Skip("no IPv6 loopback available")
	return
}

// tcpBackend accepts connections, reports each one and holds it until the peer closes.
func tcpBackend(t *testing.T, network, addr string) (port int, accepted chan struct{}) {
	t.Helper()
	l, err := net.Listen(network, addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = l.Close() })
	accepted = make(chan struct{}, 8)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			accepted <- struct{}{}
			go func() { _, _ = io.Copy(io.Discard, c); _ = c.Close() }()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port, accepted
}

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()
	return pc.LocalAddr().String()
}

// TestForwardRoutesVoiceToDialedBackend drives the real Forward: with two
// backends under round-robin (one reached through a hostname that resolves to
// several addresses), UDP from each session must land on the voice port of the
// IP its own TCP connection was dialed to.
func TestForwardRoutesVoiceToDialedBackend(t *testing.T) {
	voice4, voice6, voicePort := voiceSocketPair(t)
	tcp4Port, tcp4Accepted := tcpBackend(t, "tcp4", "127.0.0.1:0")
	tcp6Port, tcp6Accepted := tcpBackend(t, "tcp6", "[::1]:0")

	listen := freeUDPAddr(t)
	mgr := voicechat.NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, mgr.Start(ctx, logr.Discard(), []string{listen}))

	routes := []config.Route{{
		Host: configutil.SingleOrMulti[string]{"mc.test"},
		// "localhost" is a name, not an IP: the voice target must be the IP TCP dialed.
		Backend:   configutil.SingleOrMulti[string]{"localhost:" + strconv.Itoa(tcp4Port), "[::1]:" + strconv.Itoa(tcp6Port)},
		Strategy:  config.StrategyRoundRobin,
		VoiceChat: &config.VoiceChat{Listen: listen, BackendPort: voicePort},
	}}
	sm := NewStrategyManager()

	gate, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer gate.Close()
	forwarded := make(chan struct{}, 8)
	go func() {
		for {
			c, err := gate.Accept()
			if err != nil {
				return
			}
			go func() {
				mc, _ := netmc.NewMinecraftConn(ctx, c, proto.ServerBound, 30*time.Second, 30*time.Second, 0, nil)
				hs := &packet.Handshake{ServerAddress: "mc.test", ProtocolVersion: 767, NextStatus: 2}
				Forward(2*time.Second, routes, logr.Discard(), mc, hs, &proto.PacketContext{Payload: []byte{0}}, sm, mgr)
				forwarded <- struct{}{}
			}()
		}
	}()

	pkt := make([]byte, 20)
	pkt[0] = 0xFF
	hits := map[string]int{}
	for i := 0; i < 2; i++ {
		tcp, err := net.Dial("tcp", gate.Addr().String())
		require.NoError(t, err)

		var dialed string
		var want, other *voiceSocket
		select {
		case <-tcp4Accepted:
			dialed, want, other = "v4", voice4, voice6
		case <-tcp6Accepted:
			dialed, want, other = "v6", voice6, voice4
		case <-time.After(3 * time.Second):
			t.Fatal("no backend accepted the forwarded connection")
		}
		hits[dialed]++

		// The session registers right after the dial; datagrams before that are dropped.
		udp, err := net.Dial("udp", listen)
		require.NoError(t, err)
		require.Eventually(t, func() bool {
			_, _ = udp.Write(pkt)
			select {
			case <-want.got:
				return true
			case <-time.After(20 * time.Millisecond):
				return false
			}
		}, 3*time.Second, time.Millisecond, "UDP must reach the voice port of the dialed backend (%s)", dialed)
		time.Sleep(100 * time.Millisecond)
		require.Empty(t, other.got, "UDP must not reach the other backend")
		for len(want.got) > 0 {
			<-want.got
		}

		_ = udp.Close()
		_ = tcp.Close()
		select {
		case <-forwarded:
		case <-time.After(3 * time.Second):
			t.Fatal("Forward did not return after the client closed")
		}
	}
	require.Equal(t, map[string]int{"v4": 1, "v6": 1}, hits, "round-robin must use both backends")
}

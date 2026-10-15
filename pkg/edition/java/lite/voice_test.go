package lite

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/lite/voicechat"
	"go.minekube.com/gate/pkg/util/configutil"
)

func TestVoiceListens(t *testing.T) {
	routes := []config.Route{
		{Host: configutil.SingleOrMulti[string]{"a"}},
		{VoiceChat: &config.VoiceChat{Listen: ":25000"}},
		{VoiceChat: &config.VoiceChat{}},
		{VoiceChat: &config.VoiceChat{Listen: ":25000"}},
	}
	require.Equal(t, []string{":24454", ":25000"}, VoiceListens(routes))
	require.Empty(t, VoiceListens([]config.Route{{}}))
}

func TestRegisterVoiceNoopCases(t *testing.T) {
	client := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000}
	require.NotNil(t, registerVoice(nil, &config.Route{VoiceChat: &config.VoiceChat{}}, tcpAddr("127.0.0.1", 25565), client))
	m := voicechat.NewManager()
	require.NotNil(t, registerVoice(m, &config.Route{}, tcpAddr("127.0.0.1", 25565), client), "route without voiceChat")
	require.NotNil(t, registerVoice(m, &config.Route{VoiceChat: &config.VoiceChat{}}, tcpAddr("127.0.0.1", 25565), &net.UnixAddr{Name: "x"}), "non-IP client")
}

func tcpAddr(ip string, port int) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

func TestVoiceBackendAddr(t *testing.T) {
	require.Equal(t, "10.0.0.1:24500", voiceBackendAddr(netip.MustParseAddr("10.0.0.1"), 24500))
	require.Equal(t, "10.0.0.1:24500", voiceBackendAddr(netip.MustParseAddr("::ffff:10.0.0.1"), 24500), "4in6 unmapped")
	require.Equal(t, "[2001:db8::1]:24500", voiceBackendAddr(netip.MustParseAddr("2001:db8::1"), 24500))
	require.Equal(t, "[fe80::1]:24500", voiceBackendAddr(netip.MustParseAddr("fe80::1%eth0"), 24500), "zone stripped")
}

func TestClientIP(t *testing.T) {
	ip, ok := clientIP(&net.TCPAddr{IP: net.ParseIP("::ffff:203.0.113.7"), Port: 1})
	require.True(t, ok)
	require.Equal(t, "203.0.113.7", ip.String(), "4in6 must be unmapped")
	_, ok = clientIP(&net.UnixAddr{Name: "x"})
	require.False(t, ok)
}

// End to end: an IPv4-mapped TCP client address registers so its UDP datagrams
// reach the backend, and release stops them.
func TestRegisterVoiceRelaysAndReleases(t *testing.T) {
	backend, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backend.Close()
	backendPort := backend.LocalAddr().(*net.UDPAddr).Port

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	listen := pc.LocalAddr().String()
	require.NoError(t, pc.Close())

	m := voicechat.NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{listen}))

	route := &config.Route{VoiceChat: &config.VoiceChat{Listen: listen, BackendPort: backendPort}}
	client := &net.TCPAddr{IP: net.ParseIP("::ffff:127.0.0.1"), Port: 40000}
	release := registerVoice(m, route, tcpAddr("127.0.0.1", 25565), client)

	pkt := make([]byte, 20)
	pkt[0] = 0xFF
	for i := 1; i <= 16; i++ {
		pkt[i] = byte(i)
	}
	send := func() {
		c, err := net.Dial("udp", listen)
		require.NoError(t, err)
		defer c.Close()
		_, err = c.Write(pkt)
		require.NoError(t, err)
	}
	recv := func(wait time.Duration) bool {
		_ = backend.SetReadDeadline(time.Now().Add(wait))
		buf := make([]byte, 64)
		n, _, err := backend.ReadFrom(buf)
		return err == nil && n > 0
	}

	send()
	require.True(t, recv(3*time.Second), "registered client's datagram must reach the backend")

	release()
	release() // idempotent
	send()    // fresh source port
	require.False(t, recv(300*time.Millisecond), "released client must no longer be relayed")
}

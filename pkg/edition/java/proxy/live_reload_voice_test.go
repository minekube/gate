package proxy

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
)

func freeLoopbackUDP(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()
	return pc.LocalAddr().String()
}

func voiceTestProxy(t *testing.T, listen string) (*Proxy, config.Config) {
	t.Helper()
	cfg := config.DefaultConfig
	cfg.Quota.Connections.Enabled = false
	cfg.PacketLimiter.PacketsPerSecond = -1
	cfg.PacketLimiter.BytesPerSecond = -1
	cfg.Lite = liteconfig.Config{
		Enabled: true,
		Routes: []liteconfig.Route{{
			Host:      []string{"play.example.test"},
			Backend:   []string{"127.0.0.1:25565"},
			VoiceChat: &liteconfig.VoiceChat{Listen: listen},
		}},
	}
	p, err := New(Options{Config: &cfg})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, p.Lite().Voice().Start(ctx, logr.Discard(), []string{listen}))
	return p, cfg
}

func voiceCandidate(cfg config.Config, vc *liteconfig.VoiceChat) *config.Config {
	candidate := cfg
	candidate.Lite.Routes = append([]liteconfig.Route(nil), cfg.Lite.Routes...)
	candidate.Lite.Routes[0].VoiceChat = vc
	return &candidate
}

func TestApplyLiveConfigRejectsOccupiedVoiceListenAndKeepsOldRelay(t *testing.T) {
	oldListen := freeLoopbackUDP(t)
	p, cfg := voiceTestProxy(t, oldListen)
	before, generation := p.configSnapshot()

	taken, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer taken.Close()
	candidate := voiceCandidate(cfg, &liteconfig.VoiceChat{Listen: taken.LocalAddr().String()})

	require.Error(t, p.ApplyLiveConfig(candidate), "listen already bound by another socket")

	after, afterGeneration := p.configSnapshot()
	require.Same(t, before, after, "old snapshot must stay published")
	require.Equal(t, generation, afterGeneration)
	require.Equal(t, []string{oldListen}, p.Lite().Voice().Listening())

	// The old listener still relays for a registered session.
	backend, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backend.Close()
	release := p.Lite().Voice().Register(netip.MustParseAddr("127.0.0.1"), oldListen, backend.LocalAddr().String())
	defer release()
	c, err := net.Dial("udp", oldListen)
	require.NoError(t, err)
	defer c.Close()
	pkt := make([]byte, 20)
	pkt[0] = 0xFF
	_, err = c.Write(pkt)
	require.NoError(t, err)
	require.NoError(t, backend.SetReadDeadline(time.Now().Add(3*time.Second)))
	n, _, err := backend.ReadFrom(make([]byte, 64))
	require.NoError(t, err)
	require.Equal(t, len(pkt), n)
}

func TestApplyLiveConfigRemovingVoiceChatClosesListener(t *testing.T) {
	listen := freeLoopbackUDP(t)
	p, cfg := voiceTestProxy(t, listen)
	require.Equal(t, []string{listen}, p.Lite().Voice().Listening())

	require.NoError(t, p.ApplyLiveConfig(voiceCandidate(cfg, nil)))

	require.Empty(t, p.Lite().Voice().Listening())
	pc, err := net.ListenPacket("udp", listen)
	require.NoError(t, err, "port must be bindable again after the listener closed")
	_ = pc.Close()
}

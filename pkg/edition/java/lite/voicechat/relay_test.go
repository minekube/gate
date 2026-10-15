package voicechat

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

// voicePacket builds a datagram shaped like a client packet: magic, 16 byte
// "UUID" filled with id, a few payload bytes.
func voicePacket(id byte) []byte {
	b := make([]byte, 20)
	b[0] = 0xFF
	for i := 1; i < 17; i++ {
		b[i] = id
	}
	return b
}

// fakeBackend answers "ack" only for datagrams whose UUID byte equals id,
// mimicking a Simple Voice Chat server that ignores unknown UUIDs.
type fakeBackend struct {
	conn     *net.UDPConn
	id       byte
	received chan []byte
}

func newFakeBackend(t *testing.T, id byte) *fakeBackend {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	b := &fakeBackend{conn: c, id: id, received: make(chan []byte, 64)}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			p := append([]byte(nil), buf[:n]...)
			b.received <- p
			if n > 1 && p[1] == b.id {
				_, _ = c.WriteToUDP([]byte{0xFF, 'a', 'c', 'k', id}, from)
			}
		}
	}()
	return b
}

func (b *fakeBackend) addr() string { return b.conn.LocalAddr().String() }

func (b *fakeBackend) expectNothing(t *testing.T) {
	t.Helper()
	select {
	case p := <-b.received:
		t.Fatalf("backend unexpectedly received %d bytes", len(p))
	case <-time.After(150 * time.Millisecond):
	}
}

type relayEnv struct {
	relay    *Relay
	sessions *Sessions
	addr     *net.UDPAddr
}

func startRelay(t *testing.T, opts RelayOptions) *relayEnv {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	s := NewSessions()
	r := NewRelay(pc, s, opts, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Serve(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return &relayEnv{relay: r, sessions: s, addr: pc.LocalAddr().(*net.UDPAddr)}
}

func (e *relayEnv) client(t *testing.T) *net.UDPConn {
	t.Helper()
	c, err := net.DialUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}, e.addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func readReply(t *testing.T, c *net.UDPConn) []byte {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	require.NoError(t, err)
	return buf[:n]
}

var loopback = netip.MustParseAddr("127.0.0.1")

func TestRelayForwardsAndRepliesViaGate(t *testing.T) {
	env := startRelay(t, RelayOptions{})
	be := newFakeBackend(t, 7)
	env.sessions.Register(loopback, be.addr())

	c := env.client(t)
	_, err := c.Write(voicePacket(7))
	require.NoError(t, err)

	require.Equal(t, voicePacket(7), <-be.received)
	require.Equal(t, []byte{0xFF, 'a', 'c', 'k', 7}, readReply(t, c))
}

// Review focus 1: no TCP session for the source IP -> nothing reaches a backend.
func TestRelayDropsUnregisteredIP(t *testing.T) {
	env := startRelay(t, RelayOptions{})
	be := newFakeBackend(t, 7)
	// backend registered for a *different* IP only
	env.sessions.Register(netip.MustParseAddr("198.51.100.9"), be.addr())

	c := env.client(t)
	_, err := c.Write(voicePacket(7))
	require.NoError(t, err)
	be.expectNothing(t)
	require.Zero(t, env.relay.flowCount())
}

// Review focus 5: oversized and malformed datagrams are dropped without opening flows.
func TestRelayDropsMalformedDatagrams(t *testing.T) {
	env := startRelay(t, RelayOptions{})
	be := newFakeBackend(t, 7)
	env.sessions.Register(loopback, be.addr())
	c := env.client(t)

	noMagic := voicePacket(7)
	noMagic[0] = 0x00
	oversized := append(voicePacket(7), make([]byte, 2100)...)
	for name, p := range map[string][]byte{
		"no magic": noMagic, "too short": {0xFF, 1, 2}, "oversized": oversized,
	} {
		_, err := c.Write(p)
		require.NoError(t, err, name)
	}
	be.expectNothing(t)
	require.Zero(t, env.relay.flowCount())
}

// Review focus 2: two backends registered for one IP; probe both, pin to the answering one.
func TestRelayProbesCandidatesAndPins(t *testing.T) {
	env := startRelay(t, RelayOptions{})
	a, b := newFakeBackend(t, 1), newFakeBackend(t, 2)
	env.sessions.Register(loopback, a.addr())
	env.sessions.Register(loopback, b.addr())

	c := env.client(t)
	_, err := c.Write(voicePacket(2)) // only b knows this UUID
	require.NoError(t, err)
	<-a.received // probe reached a too
	<-b.received
	require.Equal(t, []byte{0xFF, 'a', 'c', 'k', 2}, readReply(t, c))

	_, err = c.Write(voicePacket(2))
	require.NoError(t, err)
	<-b.received
	a.expectNothing(t) // pinned: a no longer sees this flow
}

// Review focus 3: the pinned backend's session ended (player reconnected elsewhere).
func TestRelayDropsFlowWhenPinnedBackendSessionEnds(t *testing.T) {
	env := startRelay(t, RelayOptions{IdleTimeout: 200 * time.Millisecond})
	a, b := newFakeBackend(t, 1), newFakeBackend(t, 2)
	relA := env.sessions.Register(loopback, a.addr())
	env.sessions.Register(loopback, b.addr())

	c := env.client(t)
	_, _ = c.Write(voicePacket(1))
	require.Equal(t, []byte{0xFF, 'a', 'c', 'k', 1}, readReply(t, c))

	relA() // player left a; b still registered for the IP
	require.Eventually(t, func() bool { return env.relay.flowCount() == 0 },
		2*time.Second, 20*time.Millisecond, "stale flow pinned to a must be closed")
}

func TestRelayIdleExpiry(t *testing.T) {
	env := startRelay(t, RelayOptions{IdleTimeout: 150 * time.Millisecond})
	be := newFakeBackend(t, 7)
	env.sessions.Register(loopback, be.addr())
	c := env.client(t)
	_, _ = c.Write(voicePacket(7))
	readReply(t, c)
	require.Equal(t, 1, env.relay.flowCount())
	require.Eventually(t, func() bool { return env.relay.flowCount() == 0 },
		2*time.Second, 20*time.Millisecond)
}

// Review focus 5: flow cap per listener.
func TestRelayCapsFlows(t *testing.T) {
	env := startRelay(t, RelayOptions{MaxFlows: 2})
	be := newFakeBackend(t, 7)
	env.sessions.Register(loopback, be.addr())
	for i := 0; i < 5; i++ {
		c := env.client(t) // distinct source port each
		_, _ = c.Write(voicePacket(7))
	}
	require.Eventually(t, func() bool { return len(be.received) >= 2 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	require.Equal(t, 2, env.relay.flowCount())
	require.Len(t, be.received, 2)
}

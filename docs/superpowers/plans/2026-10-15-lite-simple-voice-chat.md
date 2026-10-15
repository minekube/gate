# Lite Simple Voice Chat Relay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let players connected through Gate Lite use the Simple Voice Chat mod by relaying its UDP traffic to the backend the player's TCP connection landed on.

**Architecture:** A new package `pkg/edition/java/lite/voicechat` holds a per-IP session table, a UDP relay (one listener per configured `listen` address, one flow per client `ip:port`), and a manager that reconciles listeners with the Lite config. `lite.Forward` registers `clientIP -> backend voice address` for the lifetime of the TCP pipe. When several backends are registered for one IP, the first datagram goes to all of them and the flow pins to the first backend that replies.

**Tech Stack:** Go 1.26, `net` UDP, `logr`, `testify`, `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-10-15-lite-simple-voice-chat-design.md`

## Global Constraints

- Opt-in per route: a route without `voiceChat` behaves exactly as before; `Route.Equal` and `cloneLiteRoutes` are JSON based, so the new field must have JSON tags.
- Datagram rules: first byte `0xFF`, at least 17 bytes (magic + 16 byte UUID), at most 2048 bytes; everything else is dropped silently.
- Never an open relay: datagrams from an IP with no registered TCP session are dropped.
- Never log payloads, UUIDs, usernames or player addresses above debug level (V(1)+ only for addresses).
- Defaults: `listen` `":24454"`, `backendPort` `24454`, flow idle timeout 30s, max 1024 flows per listener.
- Two routes sharing a `listen` must use the same `backendPort`.
- Project rule (CLAUDE.md): keep the relay hermetic in tests (loopback UDP only, no internet).
- Commit messages end with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Review Focus

1. Datagram from an IP without a TCP session (spoofing or scanning) must be dropped, and the backend must see nothing (Task 3).
2. Two players behind one NAT IP on different backends must both get working voice via the probe, and the flow must pin so later packets stop reaching the wrong backend (Task 3).
3. Player reconnects and lands on another backend: a flow pinned to the old backend must die instead of sending voice to the wrong server (Task 3).
4. Reload: removing `voiceChat` closes the listener; a failing bind rejects the reload and leaves running listeners untouched (Tasks 4, 5).
5. Flood from a registered IP using many source ports, and oversized datagrams, must hit the caps rather than exhaust sockets (Task 3).
6. IPv4-mapped IPv6 peer addresses and backends written without a port must still match (Task 5).

---

## File Structure

- Create `pkg/edition/java/lite/voicechat/sessions.go` - refcounted `clientIP -> voice backend addresses` table.
- Create `pkg/edition/java/lite/voicechat/relay.go` - UDP relay, flows, probe/pin, reaper.
- Create `pkg/edition/java/lite/voicechat/manager.go` - listener reconcile, `Register` by listen address.
- Create `pkg/edition/java/lite/voicechat/*_test.go` - tests per file.
- Modify `pkg/edition/java/lite/config/config.go` - `VoiceChat` config, defaults, validation.
- Create `pkg/edition/java/lite/voice.go` - `VoiceListens`, `registerVoice` glue (keeps `forward.go` change small).
- Modify `pkg/edition/java/lite/lite.go` - `Lite` owns the manager.
- Modify `pkg/edition/java/lite/forward.go` - extra `voice` parameter and registration hook.
- Modify `pkg/edition/java/proxy/session_client_handshake.go`, `pkg/edition/java/proxy/proxy.go` - pass manager, start and reconcile.
- Modify `.web/docs/guide/lite.md`, `pkg/configs/config-lite.yml`, root `CLAUDE.md` - docs.

---

### Task 1: Route config `voiceChat`

**Files:**
- Modify: `pkg/edition/java/lite/config/config.go`
- Test: `pkg/edition/java/lite/config/voicechat_test.go`

**Interfaces:**
- Produces: `config.VoiceChat{Listen string; BackendPort int}`, `Route.VoiceChat *VoiceChat`, `(*VoiceChat).ListenAddr() string`, `(*VoiceChat).BackendUDPPort() int`, const `DefaultVoiceChatPort = 24454`. Nil receiver safe: both methods return defaults.

- [ ] **Step 1: Write the failing tests**

```go
package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"go.minekube.com/gate/pkg/util/configutil"
)

func voiceRoute(host string, v *VoiceChat) Route {
	return Route{
		Host:      configutil.SingleOrMulti[string]{host},
		Backend:   configutil.SingleOrMulti[string]{"backend:25565"},
		VoiceChat: v,
	}
}

func TestVoiceChatDefaults(t *testing.T) {
	var nilVC *VoiceChat
	require.Equal(t, ":24454", nilVC.ListenAddr())
	require.Equal(t, 24454, nilVC.BackendUDPPort())

	v := &VoiceChat{Listen: ":30000", BackendPort: 30001}
	require.Equal(t, ":30000", v.ListenAddr())
	require.Equal(t, 30001, v.BackendUDPPort())
}

func TestValidateVoiceChat(t *testing.T) {
	tests := []struct {
		name    string
		routes  []Route
		wantErr string
	}{
		{"empty block uses defaults", []Route{voiceRoute("a", &VoiceChat{})}, ""},
		{"valid explicit", []Route{voiceRoute("a", &VoiceChat{Listen: "0.0.0.0:24454", BackendPort: 24455})}, ""},
		{"bad listen", []Route{voiceRoute("a", &VoiceChat{Listen: "nonsense"})}, "voiceChat.listen"},
		{"listen port out of range", []Route{voiceRoute("a", &VoiceChat{Listen: ":70000"})}, "voiceChat.listen"},
		{"backend port negative", []Route{voiceRoute("a", &VoiceChat{BackendPort: -1})}, "voiceChat.backendPort"},
		{"backend port too large", []Route{voiceRoute("a", &VoiceChat{BackendPort: 65536})}, "voiceChat.backendPort"},
		{"shared listen same backend port", []Route{
			voiceRoute("a", &VoiceChat{Listen: ":24454", BackendPort: 24454}),
			voiceRoute("b", &VoiceChat{Listen: ":24454"}),
		}, ""},
		{"shared listen different backend port", []Route{
			voiceRoute("a", &VoiceChat{Listen: ":24454", BackendPort: 24454}),
			voiceRoute("b", &VoiceChat{Listen: ":24454", BackendPort: 9999}),
		}, "share listen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := Config{Routes: tt.routes}.Validate()
			if tt.wantErr == "" {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			require.ErrorContains(t, errs[0], tt.wantErr)
		})
	}
}

func TestVoiceChatYAMLRoundTripAndEqual(t *testing.T) {
	const doc = `
host: a.example.com
backend: backend:25565
voiceChat:
  listen: ":24454"
  backendPort: 24455
`
	var r Route
	require.NoError(t, yaml.Unmarshal([]byte(doc), &r))
	require.NotNil(t, r.VoiceChat)
	require.Equal(t, ":24454", r.VoiceChat.Listen)
	require.Equal(t, 24455, r.VoiceChat.BackendPort)

	other := r
	other.VoiceChat = &VoiceChat{Listen: ":24454", BackendPort: 1}
	require.False(t, r.Equal(&other), "voiceChat changes must count as route changes")
	require.True(t, r.Equal(&r))
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/edition/java/lite/config/ -run 'VoiceChat' -v`
Expected: FAIL (compile error: `VoiceChat` undefined).

- [ ] **Step 3: Implement**

In `config.go` add `"net"` to the imports, add the field to `Route` (after `Strategy`):

```go
		VoiceChat         *VoiceChat `json:"voiceChat,omitempty" yaml:"voiceChat,omitempty"` // nil = disabled
```

Add after the `Status` type block:

```go
// DefaultVoiceChatPort is the default UDP port of the Simple Voice Chat mod.
const DefaultVoiceChatPort = 24454

// VoiceChat enables relaying Simple Voice Chat UDP traffic for a route.
type VoiceChat struct {
	// Listen is the public UDP address Gate binds. Default ":24454".
	Listen string `json:"listen,omitempty" yaml:"listen,omitempty"`
	// BackendPort is the UDP voice port of the backends. Default 24454.
	BackendPort int `json:"backendPort,omitempty" yaml:"backendPort,omitempty"`
}

// ListenAddr returns the configured or default UDP listen address.
func (v *VoiceChat) ListenAddr() string {
	if v == nil || v.Listen == "" {
		return ":" + strconv.Itoa(DefaultVoiceChatPort)
	}
	return v.Listen
}

// BackendUDPPort returns the configured or default backend UDP port.
func (v *VoiceChat) BackendUDPPort() int {
	if v == nil || v.BackendPort == 0 {
		return DefaultVoiceChatPort
	}
	return v.BackendPort
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

func validListen(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	p, err := strconv.Atoi(port)
	return err == nil && validPort(p)
}
```

In `Validate`, declare `voiceListens := map[string]int{}` before the route loop and, at the end of the per-route body (still inside `for i, ep := range c.Routes`), add:

```go
		if v := ep.VoiceChat; v != nil {
			if v.Listen != "" && !validListen(v.Listen) {
				e("Route %d: invalid voiceChat.listen '%s', expected host:port with port 1-65535", i, v.Listen)
			} else if v.BackendPort != 0 && !validPort(v.BackendPort) {
				e("Route %d: invalid voiceChat.backendPort %d, expected 1-65535", i, v.BackendPort)
			} else {
				listen, bp := v.ListenAddr(), v.BackendUDPPort()
				if prev, ok := voiceListens[listen]; ok && prev != bp {
					e("Route %d: routes that share listen '%s' must use the same voiceChat.backendPort", i, listen)
				}
				voiceListens[listen] = bp
			}
		}
```

Note the "no routes" early return stays above; the map declaration goes after it.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/edition/java/lite/config/ -v`
Expected: PASS (all, including pre-existing tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/edition/java/lite/config/
git commit -m "feat(lite): add voiceChat route config"
```

---

### Task 2: Session table

**Files:**
- Create: `pkg/edition/java/lite/voicechat/sessions.go`
- Test: `pkg/edition/java/lite/voicechat/sessions_test.go`

**Interfaces:**
- Produces: `type Sessions`, `NewSessions() *Sessions`, `(*Sessions).Register(ip netip.Addr, backend string) (release func())` (release is idempotent), `(*Sessions).Backends(ip netip.Addr) []string` (sorted, nil when none). `backend` is a UDP `host:port` string.

- [ ] **Step 1: Write the failing tests**

```go
package voicechat

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionsRegisterAndRelease(t *testing.T) {
	s := NewSessions()
	ip := netip.MustParseAddr("203.0.113.5")
	require.Empty(t, s.Backends(ip))

	r1 := s.Register(ip, "b:1")
	r2 := s.Register(ip, "a:1")
	r3 := s.Register(ip, "a:1") // second player, same backend, same IP
	require.Equal(t, []string{"a:1", "b:1"}, s.Backends(ip))

	r2()
	require.Equal(t, []string{"a:1", "b:1"}, s.Backends(ip), "refcounted: one session still holds a:1")
	r3()
	require.Equal(t, []string{"b:1"}, s.Backends(ip))
	r3() // idempotent: must not underflow or drop b:1
	require.Equal(t, []string{"b:1"}, s.Backends(ip))
	r1()
	require.Empty(t, s.Backends(ip))
}

func TestSessionsIsolatedPerIP(t *testing.T) {
	s := NewSessions()
	a, b := netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")
	rel := s.Register(a, "x:1")
	require.Empty(t, s.Backends(b))
	rel()
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/edition/java/lite/voicechat/ -v`
Expected: FAIL (package has no non-test files / `NewSessions` undefined).

- [ ] **Step 3: Implement**

```go
// Package voicechat relays Simple Voice Chat UDP traffic for Gate Lite.
//
// The relay is a trust boundary: it only forwards datagrams from client IPs
// that currently hold a Lite TCP session, otherwise any host could use Gate as
// an open UDP relay towards the backends.
package voicechat

import (
	"maps"
	"net/netip"
	"slices"
	"sync"
)

// Sessions tracks which voice backends a client IP is currently connected to.
type Sessions struct {
	mu   sync.Mutex
	byIP map[netip.Addr]map[string]int
}

// NewSessions returns an empty table.
func NewSessions() *Sessions {
	return &Sessions{byIP: map[netip.Addr]map[string]int{}}
}

// Register records that ip is connected to the voice backend address (UDP host:port).
// The returned release function is idempotent.
func (s *Sessions) Register(ip netip.Addr, backend string) (release func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byIP[ip]
	if m == nil {
		m = map[string]int{}
		s.byIP[ip] = m
	}
	m[backend]++

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			m := s.byIP[ip]
			if m == nil {
				return
			}
			m[backend]--
			if m[backend] <= 0 {
				delete(m, backend)
			}
			if len(m) == 0 {
				delete(s.byIP, ip)
			}
		})
	}
}

// Backends returns the sorted voice backend addresses registered for ip.
func (s *Sessions) Backends(ip netip.Addr) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.byIP[ip]
	if len(m) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(m))
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/edition/java/lite/voicechat/ -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/edition/java/lite/voicechat/
git commit -m "feat(lite): add voice chat session table"
```

---

### Task 3: UDP relay with probe and pinning

**Files:**
- Create: `pkg/edition/java/lite/voicechat/relay.go`
- Test: `pkg/edition/java/lite/voicechat/relay_test.go`

**Interfaces:**
- Consumes: `Sessions` (Task 2).
- Produces: `RelayOptions{IdleTimeout time.Duration; MaxFlows int}`, `NewRelay(conn net.PacketConn, sessions *Sessions, opts RelayOptions, log logr.Logger) *Relay`, `(*Relay).Serve(ctx context.Context) error` (blocks, closes `conn` on return), unexported `(*Relay).flowCount() int` for tests.

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/edition/java/lite/voicechat/ -run Relay -v`
Expected: FAIL (compile error: `NewRelay`, `RelayOptions` undefined).

- [ ] **Step 3: Implement `relay.go`**

```go
package voicechat

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

const (
	magicByte    = 0xFF
	minPacket    = 17 // magic byte + 16 byte player UUID
	maxPacket    = 2048
	maxReplySize = 4096

	defaultIdleTimeout = 30 * time.Second
	defaultMaxFlows    = 1024
)

// RelayOptions tunes a Relay. Zero values select the defaults.
type RelayOptions struct {
	IdleTimeout time.Duration
	MaxFlows    int
}

func (o RelayOptions) withDefaults() RelayOptions {
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = defaultIdleTimeout
	}
	if o.MaxFlows <= 0 {
		o.MaxFlows = defaultMaxFlows
	}
	return o
}

// Relay forwards Simple Voice Chat datagrams between clients and the backends
// their TCP sessions are connected to. It never inspects encrypted payloads.
type Relay struct {
	conn     net.PacketConn
	sessions *Sessions
	opts     RelayOptions
	log      logr.Logger

	mu    sync.Mutex
	flows map[netip.AddrPort]*flow
}

// NewRelay creates a relay that serves datagrams arriving on conn.
func NewRelay(conn net.PacketConn, sessions *Sessions, opts RelayOptions, log logr.Logger) *Relay {
	return &Relay{
		conn:     conn,
		sessions: sessions,
		opts:     opts.withDefaults(),
		log:      log,
		flows:    map[netip.AddrPort]*flow{},
	}
}

// Serve relays until ctx is done or the socket fails. It closes the socket.
func (r *Relay) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = r.conn.Close() }()
	go r.reapLoop(ctx)
	defer r.closeAll()

	buf := make([]byte, maxPacket+1)
	for {
		n, from, err := r.conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		r.handleClient(from, buf[:n])
	}
}

func (r *Relay) handleClient(from net.Addr, pkt []byte) {
	if len(pkt) < minPacket || len(pkt) > maxPacket || pkt[0] != magicByte {
		return
	}
	ua, ok := from.(*net.UDPAddr)
	if !ok {
		return
	}
	ap := ua.AddrPort()
	ap = netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
	if f := r.flowFor(ap); f != nil {
		f.toBackends(pkt)
	}
}

func (r *Relay) flowFor(client netip.AddrPort) *flow {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.flows[client]; ok {
		return f
	}
	if len(r.flows) >= r.opts.MaxFlows {
		return nil
	}
	backends := r.sessions.Backends(client.Addr())
	if len(backends) == 0 {
		return nil // no TCP session for this IP: never an open relay
	}
	f := newFlow(r, client, backends)
	if f == nil {
		return nil
	}
	r.flows[client] = f
	r.log.V(1).Info("voice flow opened", "client", client.String(), "candidates", len(backends))
	return f
}

func (r *Relay) flowCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.flows)
}

func (r *Relay) reapLoop(ctx context.Context) {
	t := time.NewTicker(max(r.opts.IdleTimeout/2, 10*time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			r.sweep(now)
		}
	}
}

// sweep closes flows that are idle, whose client IP has no session left, or
// that are pinned to a backend the IP is no longer connected to.
func (r *Relay) sweep(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for client, f := range r.flows {
		if f.stale(now, r.opts.IdleTimeout, r.sessions.Backends(client.Addr())) {
			f.close()
			delete(r.flows, client)
		}
	}
}

func (r *Relay) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for client, f := range r.flows {
		f.close()
		delete(r.flows, client)
	}
}

type upstream struct {
	conn    *net.UDPConn
	backend string
}

// flow is the relay state of one client ip:port.
type flow struct {
	relay  *Relay
	client netip.AddrPort
	last   atomic.Int64 // unix nanos of last traffic in either direction

	mu        sync.Mutex
	upstreams []*upstream
	pinned    *upstream
	closed    bool
}

// newFlow dials every candidate backend. It returns nil if none is reachable.
func newFlow(r *Relay, client netip.AddrPort, backends []string) *flow {
	f := &flow{relay: r, client: client}
	f.last.Store(time.Now().UnixNano())
	for _, b := range backends {
		raddr, err := net.ResolveUDPAddr("udp", b)
		if err != nil {
			r.log.V(1).Info("voice backend not resolvable", "backend", b, "error", err)
			continue
		}
		c, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			r.log.V(1).Info("voice backend dial failed", "backend", b, "error", err)
			continue
		}
		f.upstreams = append(f.upstreams, &upstream{conn: c, backend: b})
	}
	if len(f.upstreams) == 0 {
		return nil
	}
	for _, u := range f.upstreams {
		go f.readUpstream(u)
	}
	return f
}

func (f *flow) toBackends(pkt []byte) {
	f.last.Store(time.Now().UnixNano())
	f.mu.Lock()
	targets := f.upstreams
	if f.pinned != nil {
		targets = []*upstream{f.pinned}
	}
	f.mu.Unlock()
	for _, u := range targets {
		_, _ = u.conn.Write(pkt)
	}
}

func (f *flow) readUpstream(u *upstream) {
	buf := make([]byte, maxReplySize)
	to := net.UDPAddrFromAddrPort(f.client)
	for {
		n, err := u.conn.Read(buf)
		if err != nil {
			return
		}
		if !f.accept(u) {
			continue
		}
		f.last.Store(time.Now().UnixNano())
		_, _ = f.relay.conn.WriteTo(buf[:n], to)
	}
}

// accept pins the flow to u on its first reply; later replies only count for the pinned upstream.
func (f *flow) accept(u *upstream) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	if f.pinned == u {
		return true
	}
	if f.pinned != nil {
		return false
	}
	f.pinned = u
	for _, other := range f.upstreams {
		if other != u {
			_ = other.conn.Close()
		}
	}
	f.upstreams = []*upstream{u}
	return true
}

func (f *flow) stale(now time.Time, idle time.Duration, backends []string) bool {
	if len(backends) == 0 || now.Sub(time.Unix(0, f.last.Load())) > idle {
		return true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pinned != nil && !slices.Contains(backends, f.pinned.backend)
}

func (f *flow) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	for _, u := range f.upstreams {
		_ = u.conn.Close()
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/edition/java/lite/voicechat/ -race -count=3 -v`
Expected: PASS, no race reports.

- [ ] **Step 5: Commit**

```bash
git add pkg/edition/java/lite/voicechat/
git commit -m "feat(lite): add voice chat UDP relay"
```

---

### Task 4: Manager (listener lifecycle)

**Files:**
- Create: `pkg/edition/java/lite/voicechat/manager.go`
- Test: `pkg/edition/java/lite/voicechat/manager_test.go`

**Interfaces:**
- Consumes: `Sessions`, `Relay`, `NewRelay`.
- Produces: `NewManager() *Manager`; `(*Manager).Start(ctx context.Context, log logr.Logger, listens []string) error`; `(*Manager).Reconcile(listens []string) error` (no-op returning nil before `Start`; on bind failure returns the error and leaves all running listeners unchanged); `(*Manager).Register(ip netip.Addr, listen, voiceAddr string) (release func())` (no-op release when `listen` has no running relay); `(*Manager).Listening() []string` (sorted, for tests).

- [ ] **Step 1: Write the failing tests**

```go
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

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = pc.Close() }()
	return pc.LocalAddr().String()
}

func TestManagerReconcileBeforeStartIsNoop(t *testing.T) {
	m := NewManager()
	require.NoError(t, m.Reconcile([]string{"127.0.0.1:0"}))
	require.Empty(t, m.Listening())
}

func TestManagerStartReconcileAndStop(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := freeUDPAddr(t), freeUDPAddr(t)

	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))
	require.Equal(t, []string{a}, m.Listening())

	require.NoError(t, m.Reconcile([]string{a, b}))
	require.ElementsMatch(t, []string{a, b}, m.Listening())

	// Review focus 4: removing a route's voiceChat closes its listener (port is free again).
	require.NoError(t, m.Reconcile([]string{b}))
	require.Equal(t, []string{b}, m.Listening())
	pc, err := net.ListenPacket("udp", a)
	require.NoError(t, err, "removed listener must release its port")
	_ = pc.Close()

	cancel()
	require.Eventually(t, func() bool {
		pc, err := net.ListenPacket("udp", b)
		if err != nil {
			return false
		}
		_ = pc.Close()
		return true
	}, 2*time.Second, 20*time.Millisecond, "ctx cancel must release all listeners")
}

// Review focus 4: a failing bind rejects the whole reconcile and keeps what runs.
func TestManagerBindFailureLeavesStateUnchanged(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := freeUDPAddr(t)
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))

	busy, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = busy.Close() }()

	err = m.Reconcile([]string{busy.LocalAddr().String()}) // would also remove a
	require.Error(t, err)
	require.Equal(t, []string{a}, m.Listening(), "a must keep running")
}

func TestManagerRegisterRoutesToRelaySessions(t *testing.T) {
	m := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := freeUDPAddr(t)
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{a}))
	be := newFakeBackend(t, 9)

	release := m.Register(loopback, a, be.addr())
	c, err := net.Dial("udp", a)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	_, _ = c.Write(voicePacket(9))
	require.Equal(t, voicePacket(9), <-be.received)

	release()
	// unknown listen: harmless no-op
	m.Register(netip.MustParseAddr("192.0.2.1"), "127.0.0.1:1", "x:1")()
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/edition/java/lite/voicechat/ -run Manager -v`
Expected: FAIL (`NewManager` undefined).

- [ ] **Step 3: Implement**

```go
package voicechat

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"slices"
	"sync"

	"github.com/go-logr/logr"
)

// Manager owns one Relay per distinct listen address and keeps that set in
// sync with the Lite configuration.
type Manager struct {
	mu     sync.Mutex
	ctx    context.Context // nil until Start
	log    logr.Logger
	relays map[string]*managedRelay
}

type managedRelay struct {
	relay    *Relay
	sessions *Sessions
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewManager returns a manager that does nothing until Start is called.
func NewManager() *Manager {
	return &Manager{relays: map[string]*managedRelay{}}
}

// Start binds the given listen addresses. Relays stop when ctx is done.
func (m *Manager) Start(ctx context.Context, log logr.Logger, listens []string) error {
	m.mu.Lock()
	m.ctx, m.log = ctx, log
	m.mu.Unlock()
	return m.Reconcile(listens)
}

// Reconcile makes the running listeners match listens. Before Start it does
// nothing. If any new address fails to bind, nothing is changed.
func (m *Manager) Reconcile(listens []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return nil
	}
	want := map[string]bool{}
	for _, l := range listens {
		want[l] = true
	}

	// Bind everything new first so a failure leaves the running state intact.
	bound := map[string]net.PacketConn{}
	for l := range want {
		if _, ok := m.relays[l]; ok {
			continue
		}
		pc, err := net.ListenPacket("udp", l)
		if err != nil {
			for _, p := range bound {
				_ = p.Close()
			}
			return fmt.Errorf("voice chat: listen on %s: %w", l, err)
		}
		bound[l] = pc
	}

	for l, mr := range m.relays {
		if !want[l] {
			mr.cancel()
			<-mr.done
			delete(m.relays, l)
			m.log.Info("voice chat relay stopped", "listen", l)
		}
	}
	for l, pc := range bound {
		s := NewSessions()
		r := NewRelay(pc, s, RelayOptions{}, m.log.WithValues("listen", l))
		ctx, cancel := context.WithCancel(m.ctx)
		mr := &managedRelay{relay: r, sessions: s, cancel: cancel, done: make(chan struct{})}
		go func() { defer close(mr.done); _ = r.Serve(ctx) }()
		m.relays[l] = mr
		m.log.Info("voice chat relay listening", "listen", l)
	}
	return nil
}

// Register records a TCP session of ip for the relay bound to listen.
// It is a no-op (with a no-op release) if no relay listens there.
func (m *Manager) Register(ip netip.Addr, listen, voiceAddr string) (release func()) {
	m.mu.Lock()
	mr := m.relays[listen]
	m.mu.Unlock()
	if mr == nil {
		return func() {}
	}
	return mr.sessions.Register(ip, voiceAddr)
}

// Listening returns the sorted listen addresses of running relays.
func (m *Manager) Listening() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Sorted(maps.Keys(m.relays))
}
```

Caveat to handle in the test run: `Listening()` keys are the configured strings, so tests that pass `127.0.0.1:port` compare equal.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./pkg/edition/java/lite/voicechat/ -race -count=2 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/edition/java/lite/voicechat/
git commit -m "feat(lite): manage voice chat relay listeners"
```

---

### Task 5: Wire into Lite and the proxy

**Files:**
- Create: `pkg/edition/java/lite/voice.go`
- Test: `pkg/edition/java/lite/voice_test.go`
- Modify: `pkg/edition/java/lite/lite.go`, `pkg/edition/java/lite/forward.go:40-98`, `pkg/edition/java/proxy/session_client_handshake.go:133`, `pkg/edition/java/proxy/proxy.go` (`Start` near line 259, `ApplyLiveConfig` near line 330)

**Interfaces:**
- Consumes: `voicechat.Manager` (Task 4), `config.VoiceChat` (Task 1).
- Produces: `lite.VoiceListens(routes []config.Route) []string` (deduped, sorted listen addresses of routes with `VoiceChat != nil`); unexported `registerVoice(voice *voicechat.Manager, route *config.Route, backendAddr string, client net.Addr) (release func())` (never returns nil); `(*Lite).Voice() *voicechat.Manager`; `Forward(..., strategyManager *StrategyManager, voice *voicechat.Manager)`.

- [ ] **Step 1: Write the failing tests**

```go
package lite

import (
	"context"
	"net"
	"testing"

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
	require.NotNil(t, registerVoice(nil, &config.Route{VoiceChat: &config.VoiceChat{}}, "b:25565", client))
	m := voicechat.NewManager()
	require.NotNil(t, registerVoice(m, &config.Route{}, "b:25565", client), "route without voiceChat")
	require.NotNil(t, registerVoice(m, &config.Route{VoiceChat: &config.VoiceChat{}}, "b:25565", &net.UnixAddr{Name: "x"}), "non-IP client")
}

// Review focus 6: IPv4-mapped IPv6 clients and port-less backends must still match.
func TestRegisterVoiceAddressNormalization(t *testing.T) {
	m := voicechat.NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	listen := pc.LocalAddr().String()
	_ = pc.Close()
	require.NoError(t, m.Start(ctx, logr.Discard(), []string{listen}))

	route := &config.Route{VoiceChat: &config.VoiceChat{Listen: listen, BackendPort: 24500}}
	for _, backend := range []string{"10.0.0.1:25565", "10.0.0.1"} {
		client := &net.TCPAddr{IP: net.ParseIP("::ffff:203.0.113.7"), Port: 40000}
		release := registerVoice(m, route, backend, client)
		// The relay-level behaviour is covered in voicechat; here we assert it
		// registered without panicking and the release is callable twice.
		release()
		release()
	}
}
```

To make the normalization observable, also expose in `voice.go` two pure helpers and test them directly - append to the test file:

```go
func TestVoiceBackendAddr(t *testing.T) {
	require.Equal(t, "10.0.0.1:24500", voiceBackendAddr("10.0.0.1:25565", 24500))
	require.Equal(t, "10.0.0.1:24500", voiceBackendAddr("10.0.0.1", 24500))
	require.Equal(t, "[2001:db8::1]:24500", voiceBackendAddr("[2001:db8::1]:25565", 24500))
	require.Equal(t, "mc.example.com:24500", voiceBackendAddr("mc.example.com:25565", 24500))
}

func TestClientIP(t *testing.T) {
	ip, ok := clientIP(&net.TCPAddr{IP: net.ParseIP("::ffff:203.0.113.7"), Port: 1})
	require.True(t, ok)
	require.Equal(t, "203.0.113.7", ip.String(), "4in6 must be unmapped")
	_, ok = clientIP(&net.UnixAddr{Name: "x"})
	require.False(t, ok)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./pkg/edition/java/lite/ -run 'Voice|ClientIP' -v`
Expected: FAIL (`VoiceListens` undefined).

- [ ] **Step 3: Implement**

`pkg/edition/java/lite/voice.go`:

```go
package lite

import (
	"net"
	"net/netip"
	"slices"
	"strconv"

	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/lite/voicechat"
)

// VoiceListens returns the distinct, sorted UDP listen addresses of all routes with voiceChat enabled.
func VoiceListens(routes []config.Route) []string {
	var out []string
	for i := range routes {
		if v := routes[i].VoiceChat; v != nil {
			out = append(out, v.ListenAddr())
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// registerVoice records the player's TCP session so the voice relay may forward
// that client IP's UDP traffic to the selected backend. It never returns nil.
func registerVoice(voice *voicechat.Manager, route *config.Route, backendAddr string, client net.Addr) func() {
	noop := func() {}
	if voice == nil || route == nil || route.VoiceChat == nil {
		return noop
	}
	ip, ok := clientIP(client)
	if !ok {
		return noop
	}
	return voice.Register(ip, route.VoiceChat.ListenAddr(), voiceBackendAddr(backendAddr, route.VoiceChat.BackendUDPPort()))
}

// voiceBackendAddr swaps the TCP port of a backend address for the voice UDP port.
func voiceBackendAddr(backendAddr string, port int) string {
	host, _, err := net.SplitHostPort(backendAddr)
	if err != nil {
		host = backendAddr // no port given
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func clientIP(addr net.Addr) (netip.Addr, bool) {
	var ip net.IP
	switch a := addr.(type) {
	case *net.TCPAddr:
		ip = a.IP
	case *net.UDPAddr:
		ip = a.IP
	default:
		return netip.Addr{}, false
	}
	nip, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return nip.Unmap(), true
}
```

`lite.go`: add the import `"go.minekube.com/gate/pkg/edition/java/lite/voicechat"`, a field `voice *voicechat.Manager`, set `voice: voicechat.NewManager()` in `NewLite`, and:

```go
// Voice returns the Simple Voice Chat relay manager.
func (l *Lite) Voice() *voicechat.Manager { return l.voice }
```

`forward.go`: add import `"go.minekube.com/gate/pkg/edition/java/lite/voicechat"`, extend the signature with `voice *voicechat.Manager` after `strategyManager *StrategyManager`, and after the `defer decrementConnection()` line add:

```go
	defer registerVoice(voice, route, backendAddr, src.RemoteAddr())()
```

`session_client_handshake.go:133`: append `, h.proxy.Lite().Voice()` to the `lite.Forward(...)` call.

`proxy.go` `Start`: directly after `logInfo()` add

```go
	if cfg := p.config(); cfg.Lite.Enabled {
		if err := p.lite.Voice().Start(ctx, p.log.WithName("lite").WithName("voicechat"), lite.VoiceListens(cfg.Lite.Routes)); err != nil {
			return err
		}
	}
```

`proxy.go` `ApplyLiveConfig`: after `routes, err := cloneLiteRoutes(...)` error check and before `published := *current`, add

```go
	if err := p.lite.Voice().Reconcile(lite.VoiceListens(routes)); err != nil {
		p.log.Error(err, "rejected live config: voice chat listener")
		return errors.New("invalid live configuration")
	}
```

(`Reconcile` is a no-op before `Start`, so existing `ApplyLiveConfig` tests are unaffected.)

- [ ] **Step 4: Run to verify pass**

Run: `go build ./... && go test ./pkg/edition/java/lite/... ./pkg/edition/java/proxy/... -race`
Expected: build OK, PASS. If another caller of `lite.Forward` exists (grep `lite.Forward(` / `Forward(` in `pkg`), pass `nil` for `voice`.

- [ ] **Step 5: Commit**

```bash
git add pkg/edition/java/lite pkg/edition/java/proxy
git commit -m "feat(lite): relay Simple Voice Chat UDP for routes with voiceChat"
```

---

### Task 6: Docs, shipped config, agent note, full verification

**Files:**
- Modify: `.web/docs/guide/lite.md` (new section before `## Complete Lite config`)
- Modify: `pkg/configs/config-lite.yml` (commented example only)
- Modify: `CLAUDE.md` (one concise bullet)

- [ ] **Step 1: Docs section** in `.web/docs/guide/lite.md`:

````markdown
## Simple Voice Chat

Gate Lite can relay the UDP traffic of the [Simple Voice Chat](https://github.com/henkelmax/simple-voice-chat) mod.
Enable it per route:

```yaml
lite:
  enabled: true
  routes:
    - host: play.example.com
      backend: [10.0.0.1:25565, 10.0.0.2:25565]
      strategy: round-robin
      voiceChat:
        listen: ":24454"     # public UDP address Gate binds (default ":24454")
        backendPort: 24454   # UDP voice port of the backends (default 24454)
```

How it works: a client's UDP packets are forwarded to the backend its TCP connection is connected to, matched by
client IP. Packets from IPs without an active connection are dropped. If several players share one IP and are on
different backends, Gate probes the candidates and pins each voice connection to the backend that answers.

Setup checklist:

- Open the `listen` UDP port on Gate.
- On every backend set `voice_host` in `voicechat-server.properties` to Gate's public address (for example
  `play.example.com:24454`), otherwise clients connect to the backend directly.
- Gate must reach the backends on UDP `backendPort`.
- Routes may share a `listen` address if they use the same `backendPort`.
````

- [ ] **Step 2: Shipped config** — in `pkg/configs/config-lite.yml`, below the `fallback` block of the first route, add commented lines only (the file must stay valid for `TestStartupShippedConfigs`):

```yaml
        # Optional Simple Voice Chat UDP relay, see https://gate.minekube.com/guide/lite#simple-voice-chat
        #voiceChat:
        #  listen: ":24454"
        #  backendPort: 24454
```

- [ ] **Step 3: CLAUDE.md bullet** (root, before "## Maintaining this file"):

```markdown
- Lite's Simple Voice Chat UDP relay (`pkg/edition/java/lite/voicechat`) is a trust boundary like PROXY protocol: it forwards only datagrams from client IPs holding a live Lite TCP session (`Sessions`), otherwise Gate is an open UDP relay into the backends. Never log payloads or UUIDs; routing is by client IP with probe-and-pin for IPs shared across backends. Regression coverage: `voicechat/relay_test.go`.
```

- [ ] **Step 4: Full verification**

Run: `go vet ./... && go test ./... -race 2>&1 | tail -30`
Expected: PASS everywhere (this includes `pkg/gate/gate_startup_test.go`). Then run the real-client check from the spec's open item manually: start Gate Lite with one route and `voiceChat`, point a Minecraft client with the mod at it with `voice_host` set on the backend, and confirm voice works; also test once with `voice_host` empty and note the result in the PR body. If a real client is unavailable, say so explicitly in the PR body.

- [ ] **Step 5: Commit**

```bash
git add .web/docs/guide/lite.md pkg/configs/config-lite.yml CLAUDE.md
git commit -m "docs(lite): document Simple Voice Chat relay"
```

---

## Self-Review

- **Spec coverage:** config + validation (T1), session table (T2), relay with probe/pin, drop rules, idle, caps, stale-flow close (T3), listener lifecycle and reload reconcile with rejected bind (T4, T5), Forward hook and PROXY-protocol-correct peer address via `src.RemoteAddr()` (T5), operator notes and docs (T6), tests listed in spec (T1-T5). The spec's "validate ports" is relaxed to "defaults when zero, strict when set".
- **Placeholder scan:** none; every code step has code.
- **Type consistency:** `Sessions.Register(ip, backend)`, `Manager.Register(ip, listen, voiceAddr)`, `registerVoice(voice, route, backendAddr, client)`, `NewRelay(conn, sessions, opts, log)` are used identically across tasks.
- **Known limits (documented, not hidden):** backend hostnames are resolved when a flow opens, on the relay's read path; a stalled DNS lookup delays other datagrams on that listener for the resolver timeout. Acceptable for v1, mention in PR.

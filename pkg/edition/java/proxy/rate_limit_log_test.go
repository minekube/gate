package proxy

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/loopback"
)

// capturedLogs records what the proxy would have written, so a test can assert
// that a limiter rejection is actually visible to an operator.
type capturedLogs struct {
	mu    sync.Mutex
	lines []string
}

func (c *capturedLogs) printf(prefix, args string) {
	c.mu.Lock()
	c.lines = append(c.lines, prefix+args)
	c.mu.Unlock()
}

func (c *capturedLogs) contains(substr string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, line := range c.lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

type rateLimitAttempt struct {
	kicked            bool
	kickPayload       string
	loginSuccess      bool
	encryptionRequest bool
	closedWithoutData bool
}

// newRateLimitProxy builds a real proxy whose log output is captured. The
// listener is real TCP on 127.0.0.1 because the quota buckets by IP prefix -
// net.Pipe's "pipe" address would never map into a bucket and silently disable
// both limiters - and it comes from the loopback allocator rather than the
// kernel's ephemeral range (pkg/util/loopback, see this package's AGENTS.md).
func newRateLimitProxy(t *testing.T, quota config.Quota) (*Proxy, net.Listener, *capturedLogs) {
	t.Helper()

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:1"}
	cfg.Try = []string{"lobby"}
	cfg.Quota = quota

	p, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}

	logs := &capturedLogs{}
	p.log = funcr.New(logs.printf, funcr.Options{Verbosity: 1})

	if err := p.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}

	alloc, err := loopback.Default()
	if err != nil {
		t.Fatalf("loopback allocator error: %v", err)
	}
	ln, err := alloc.Listen()
	if err != nil {
		t.Fatalf("listen error: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return p, ln, logs
}

// runLogin drives one handshake+login through the real HandleConn entry point.
func runLogin(t *testing.T, p *Proxy, ln net.Listener, username string) rateLimitAttempt {
	t.Helper()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer client.Close()

	server, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.HandleConn(server)
	}()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("HandleConn did not return after the client connection was closed")
		}
	})

	// A limiter may close the connection before these writes land, so write
	// errors are not fatal: whatever the proxy already queued is read below.
	_ = writeTunnelHandshake(client, "127.0.0.1", 25565, int(version.Minecraft_1_20.Protocol))
	_ = writeServerLogin(client, username)

	var outcome rateLimitAttempt
	deadline := time.Now().Add(10 * time.Second)
	var readAny bool
	for {
		_ = client.SetReadDeadline(deadline)
		packetID, data, err := readPacket(client)
		if err != nil {
			break
		}
		readAny = true
		switch packetID {
		case 0x00: // login-state Disconnect
			outcome.kicked = true
			outcome.kickPayload = string(data)
		case 0x01: // login-state EncryptionRequest
			outcome.encryptionRequest = true
		case 0x02: // login-state LoginSuccess
			outcome.loginSuccess = true
		}
		if outcome.kicked || outcome.loginSuccess || outcome.encryptionRequest {
			break
		}
	}
	outcome.closedWithoutData = !readAny
	return outcome
}

// TestLoginRateLimitIsLogged pins the operator-facing behaviour the reporter
// asked for: when the per-IP-block login limiter drops a session, the proxy
// must say so. Before the fix the login limiter closed the connection with only
// a client-side kick message, so an operator saw players complaining about
// "logging in too fast" with nothing in the log to correlate.
func TestLoginRateLimitIsLogged(t *testing.T) {
	p, ln, logs := newRateLimitProxy(t, config.Quota{
		Logins: config.QuotaSettings{Enabled: true, OPS: 0.001, Burst: 1, MaxEntries: 100},
	})

	// First login from this IP block passes and consumes the burst.
	first := runLogin(t, p, ln, "LimiterFirst")
	if first.kicked {
		t.Fatalf("first login must not be rate limited, kick payload: %q", first.kickPayload)
	}
	if !first.encryptionRequest && !first.loginSuccess {
		t.Fatalf("first login did not reach authentication: %+v", first)
	}

	// Second login from the same block is denied and - the point of this test -
	// the operator gets a log line about it.
	second := runLogin(t, p, ln, "LimiterSecond")
	if !logs.contains("login exceeded rate limit") {
		t.Fatalf("rate-limited login was not logged; outcome %+v; captured log lines: %v", second, logs.lines)
	}
	// The address must be in the line, otherwise operators cannot attribute it.
	if !logs.contains("127.0.0.1:") {
		t.Fatalf("log line does not carry the remote address; captured log lines: %v", logs.lines)
	}
	// And the player must actually see why. Gate's own connection is still in the
	// handshake state at this point, whose clientbound registry has no Disconnect
	// packet, so without moving to the login state the reason never encodes and
	// the client only sees a reset.
	if !second.kicked {
		t.Fatalf("second login was not kicked with the rate-limit reason: %+v", second)
	}
	if !strings.Contains(second.kickPayload, "logging in too fast") {
		t.Fatalf("kick payload = %q, want the rate-limit disconnect reason", second.kickPayload)
	}
}

// TestConnectionRateLimitIsLogged keeps the existing connection-limiter log
// covered by the same harness, so the two limiter exits cannot drift apart.
func TestConnectionRateLimitIsLogged(t *testing.T) {
	p, ln, logs := newRateLimitProxy(t, config.Quota{
		Connections: config.QuotaSettings{Enabled: true, OPS: 0.001, Burst: 1, MaxEntries: 100},
	})

	first := runLogin(t, p, ln, "ConnLimiterFirst")
	if first.kicked {
		t.Fatalf("first connection must not be rate limited, kick payload: %q", first.kickPayload)
	}

	second := runLogin(t, p, ln, "ConnLimiterSecond")
	if !second.closedWithoutData {
		t.Fatalf("second connection must be closed without any packet, got %+v", second)
	}
	if !logs.contains("connection exceeded rate limit, closed") {
		t.Fatalf("rate-limited connection was not logged; captured log lines: %v", logs.lines)
	}
}

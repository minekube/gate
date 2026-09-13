package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/robinbraemer/event"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// connectBlacklistReservedName is reserved in the blacklist used below.
const connectBlacklistReservedName = "ReservedOwner"

// connectBlacklistReservedReason must survive to the client, proving the
// reserved-name kick (not some other disconnect) closed the connection.
const connectBlacklistReservedReason = "connect-scoped reservation test"

// connectBlacklistTunnelConn stands in for the connection Gate's Connect
// adapter hands to Proxy.HandleConn: the outermost tunnel wrapper is a plain
// net.Conn that only exposes the trusted Connect ingress provenance marker
// (pkg/util/connectutil/config/tunnelConnWithSession.IsConnectTunnelIngress).
// It carries no address and no handshake, so the marker is the only thing that
// can tell the login path this connection arrived through Connect.
type connectBlacklistTunnelConn struct {
	net.Conn
}

func (connectBlacklistTunnelConn) IsConnectTunnelIngress() bool { return true }

// TestConnectTunnelReservedNameKickThroughHandleConn is the end-to-end
// regression for the reserved-name blacklist on Connect ingress
// (minekube/gate, v0.72.0 - v0.73.11).
//
// offlineModeUsernameBlacklistScope: connect is a security-facing setting: it
// can only ever fire when the login decision sees the trusted Connect ingress
// marker, and that marker is only reachable by probing through every wrapper
// Gate layers onto the accepted connection. Proxy.handleConn always attaches
// the connectiontelemetry byte counter, and a ConnectionEvent subscriber adds
// connwrap.Conn on top; both embed net.Conn, which promotes net.Conn's methods
// and hides every other interface the accepted tunnel connection implements.
//
// Before the fix these wrappers had no accessor at all, so
// connectTunnelIngress() was false for EVERY Connect tunnel session and a
// connect-scoped reservation silently let offline players through with a
// reserved name. This test drives a real login through Proxy.HandleConn for a
// Connect-shaped connection and requires the kick, so the next net.Conn
// embedding wrapper cannot re-introduce the fail-open state unnoticed.
func TestConnectTunnelReservedNameKickThroughHandleConn(t *testing.T) {
	t.Run("a Connect tunnel cannot log in with a reserved offline name", func(t *testing.T) {
		p := newConnectBlacklistTestProxy(t, nil)

		packetID, data := connectBlacklistLogin(t, p, func(server net.Conn) net.Conn {
			return &connectBlacklistTunnelConn{Conn: server}
		})
		connectBlacklistRequireKick(t, packetID, data)
	})

	t.Run("online mode forcing an offline login still kicks a Connect tunnel", func(t *testing.T) {
		// The reporter's proxy runs onlineMode: true, where only a pre-login
		// decision can classify the login as offline (ForceOfflineModePreLogin,
		// the same result the Bedrock front door produces). The reservation must
		// still be enforced on Connect ingress.
		p := newConnectBlacklistTestProxy(t, func(cfg *config.Config, events event.Manager) {
			cfg.OnlineMode = true
			t.Cleanup(event.Subscribe(events, 0, func(e *PreLoginEvent) {
				e.ForceOfflineMode()
			}))
		})

		packetID, data := connectBlacklistLogin(t, p, func(server net.Conn) net.Conn {
			return &connectBlacklistTunnelConn{Conn: server}
		})
		connectBlacklistRequireKick(t, packetID, data)
	})

	t.Run("a direct connection is not affected by the connect scope", func(t *testing.T) {
		p := newConnectBlacklistTestProxy(t, nil)

		// The same name over a plain socket is a direct (untrusted-provenance)
		// offline login, which this operator deliberately allows.
		packetID, _ := connectBlacklistLogin(t, p, func(server net.Conn) net.Conn { return server })
		if packetID != connectBlacklistPacketLoginSuccess {
			t.Fatalf("direct login answered with packet 0x%02x, want LoginSuccess 0x%02x; "+
				"scope: connect must only reserve names on Connect ingress",
				packetID, connectBlacklistPacketLoginSuccess)
		}
	})
}

const (
	connectBlacklistPacketDisconnect   = 0x00 // login-state Disconnect
	connectBlacklistPacketLoginSuccess = 0x02 // login-state LoginSuccess
)

// connectBlacklistRequireKick asserts the proxy answered the login with the
// configured reserved-name disconnect instead of completing it.
func connectBlacklistRequireKick(t *testing.T, packetID int, data []byte) {
	t.Helper()
	if packetID != connectBlacklistPacketDisconnect {
		t.Fatalf("proxy answered a reserved name on Connect ingress with login packet 0x%02x, "+
			"want Disconnect 0x%02x: offlineModeUsernameBlacklistScope: connect was inert, so the "+
			"reserved name was admitted (fail-open)", packetID, connectBlacklistPacketDisconnect)
	}
	var reason string
	if err := json.Unmarshal(data, &reason); err != nil {
		// The reason may carry a component object rather than a bare string.
		reason = string(data)
	}
	if !strings.Contains(reason, connectBlacklistReservedReason) {
		t.Fatalf("disconnect reason = %q, want it to contain %q", reason, connectBlacklistReservedReason)
	}
}

// newConnectBlacklistTestProxy builds a proxy with a Connect-scoped
// reservation. configure may adjust the config and subscribe to events before
// the proxy is created.
func newConnectBlacklistTestProxy(t *testing.T, configure func(*config.Config, event.Manager)) *Proxy {
	t.Helper()

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:1"}
	cfg.Try = []string{"lobby"}
	cfg.Quota.Connections.Enabled = false
	cfg.Quota.Logins.Enabled = false
	cfg.PacketLimiter.PacketsPerSecond = -1
	cfg.PacketLimiter.BytesPerSecond = -1
	cfg.OfflineModeUsernameBlacklist = []string{connectBlacklistReservedName}
	cfg.OfflineModeUsernameBlacklistScope = config.OfflineModeUsernameBlacklistScopeConnect
	reason := configutil.TextComponent(component.Text{Content: connectBlacklistReservedReason})
	cfg.OfflineModeUsernameBlacklistReason = &reason

	// A real event manager with a ConnectionEvent subscriber reproduces the
	// production wrapper order exactly: handleConn attaches the telemetry byte
	// counter, then ConnectionEvent's close tracker wraps it.
	events := event.New(event.WithRecoverPanic(false))
	t.Cleanup(event.Subscribe(events, 0, func(*ConnectionEvent) {}))

	if configure != nil {
		configure(&cfg, events)
	}

	p, err := New(Options{Config: &cfg, EventMgr: events})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}
	// Proxy goroutines can outlive the test body, so stop logging with it.
	var testDone atomic.Bool
	t.Cleanup(func() { testDone.Store(true) })
	p.log = funcr.New(func(prefix, args string) {
		if !testDone.Load() {
			t.Logf("PROXY: %s %s", prefix, args)
		}
	}, funcr.Options{Verbosity: 1})
	if err := p.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}
	return p
}

// connectBlacklistLogin performs a handshake + ServerLogin through
// Proxy.HandleConn and returns the packet ID and payload of the proxy's first
// login-state answer. wrap turns the accepted server side of the pipe into the
// connection shape under test.
func connectBlacklistLogin(t *testing.T, p *Proxy, wrap func(net.Conn) net.Conn) (int, []byte) {
	t.Helper()

	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.HandleConn(wrap(server))
	}()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("HandleConn did not return after the client connection was closed")
		}
	})

	if err := connectBlacklistWriteHandshake(client, "connect-tunnel.test", 25565, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client: failed to send handshake: %v", err)
	}
	// Lower case: the reservation is case-insensitive on every login path.
	if err := connectBlacklistWriteServerLogin(client, strings.ToLower(connectBlacklistReservedName)); err != nil {
		t.Fatalf("client: failed to send ServerLogin: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(10 * time.Second))
	packetID, data, err := connectBlacklistReadPacket(client)
	if err != nil {
		t.Fatalf("client: read error before the proxy answered login: %v", err)
	}
	return packetID, data
}

// --- self-contained wire helpers (the tag under test may not carry the ones in
// the newer integration tests) ---

func connectBlacklistWriteHandshake(w io.Writer, host string, port, protocolVersion int) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // Handshake packet ID
	pw.VarInt(protocolVersion)
	pw.String(host + "\x00" + "127.0.0.1")
	_ = util.WriteUint16(&payload, uint16(port))
	pw.VarInt(2) // Login intent
	return connectBlacklistWriteFrame(w, payload.Bytes())
}

func connectBlacklistWriteServerLogin(w io.Writer, username string) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // ServerLogin packet ID
	pw.String(username)
	pw.Bool(true) // hasUUID
	_ = util.WriteUUID(&payload, uuid.New())
	return connectBlacklistWriteFrame(w, payload.Bytes())
}

func connectBlacklistWriteFrame(w io.Writer, payload []byte) error {
	var frame bytes.Buffer
	util.PanicWriter(&frame).VarInt(len(payload))
	frame.Write(payload)
	_, err := w.Write(frame.Bytes())
	return err
}

func connectBlacklistReadPacket(r io.Reader) (int, []byte, error) {
	length, err := connectBlacklistReadVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length <= 0 || length > 1048576 {
		return 0, nil, io.ErrUnexpectedEOF
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	reader := bytes.NewReader(payload)
	packetID, err := connectBlacklistReadVarInt(reader)
	if err != nil {
		return 0, nil, err
	}
	data := make([]byte, reader.Len())
	_, _ = reader.Read(data)
	return packetID, data, nil
}

func connectBlacklistReadVarInt(r io.Reader) (int, error) {
	var result int
	var shift uint
	buf := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		result |= int(buf[0]&0x7F) << shift
		if buf[0]&0x80 == 0 {
			return result, nil
		}
		shift += 7
		if shift >= 35 {
			return 0, io.ErrUnexpectedEOF
		}
	}
}

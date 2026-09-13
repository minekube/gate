package proxy

import (
	"bytes"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/internal/connwrap"
	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
	"go.minekube.com/gate/pkg/util/uuid"
)

// tunnelProfileConn stands in for Connect's wrapTunnelSession connection: a
// plain net.Conn that additionally carries the authenticated game profile (and
// the trusted ingress marker) the login decision must be able to see. It is
// deliberately opaque to every wrapper in between, which may only probe it
// through netmc.Assert.
type tunnelProfileConn struct {
	net.Conn
	gp *profile.GameProfile
}

func (c *tunnelProfileConn) GameProfile() *profile.GameProfile { return c.gp }
func (c *tunnelProfileConn) IsConnectTunnelIngress() bool      { return true }

// TestProductionConnectionWrappersPreserveConnectTunnelInterfaces guards the
// wrappers Gate layers between an accepted connection and the Minecraft
// decoder. They embed net.Conn to count bytes and track Close, and that
// embedding hides every other interface the accepted connection carries: the
// login decision only reaches a Connect tunnel's GameProfileProvider and
// ConnectTunnelIngress through netmc.Assert, which descends through the
// wrapper chain and nowhere else.
//
// Regression for minekube/gate#1081.
func TestProductionConnectionWrappersPreserveConnectTunnelInterfaces(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	gp := &profile.GameProfile{ID: uuid.New(), Name: "TunnelVerified"}
	// The exact production order in Proxy.handleConn: the telemetry byte
	// counter is attached first, ConnectionEvent's close tracker wraps it.
	wire := connectiontelemetry.Wrap(&tunnelProfileConn{Conn: server, gp: gp})
	stack := &connwrap.Conn{Conn: wire}

	got, ok := netmc.Assert[GameProfileProvider](stack)
	if !ok {
		t.Fatalf("netmc.Assert[GameProfileProvider] failed on the production wrapper stack %T -> %T; "+
			"the wrappers mask the profile the Connect tunnel supplied", stack, wire)
	}
	if got.GameProfile() != gp {
		t.Fatalf("GameProfile() = %#v, want the tunnel-supplied %#v", got.GameProfile(), gp)
	}

	ingress, ok := netmc.Assert[ConnectTunnelIngress](stack)
	if !ok || !ingress.IsConnectTunnelIngress() {
		t.Fatalf("netmc.Assert[ConnectTunnelIngress] = (%v, %v) on the production wrapper stack; "+
			"the trusted Connect ingress marker must survive wrapping", ingress, ok)
	}

	// Byte accounting must stay on the wire boundary.
	go func() { _, _ = client.Write([]byte("payload")) }()
	buf := make([]byte, len("payload"))
	if _, err := io.ReadFull(stack, buf); err != nil {
		t.Fatalf("read through the wrapper stack: %v", err)
	}
	if read, _ := wire.Bytes(); read != int64(len("payload")) {
		t.Fatalf("tracked read bytes = %d, want %d", read, len("payload"))
	}
}

// TestConnectTunnelLoginUsesSuppliedGameProfile is the wire-level reproduction
// of minekube/gate#1081: a premium Connect tunnel hands Gate an authenticated
// game profile on the tunnel connection, but the telemetry wrapper masked it,
// so the login decision missed netmc.Assert[GameProfileProvider] and pushed an
// EncryptionRequest back into the tunnel instead of completing the login with
// the supplied profile (the client then EOFs and the session hangs).
func TestConnectTunnelLoginUsesSuppliedGameProfile(t *testing.T) {
	const username = "TunnelPremium"

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = true // the tunnel already authenticated the player
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1 // no SetCompression frame in the wire assertions
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:1"}
	cfg.Try = []string{"lobby"}

	p, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}
	// Proxy goroutines can outlive the test body, so stop logging with it.
	var testDone atomic.Bool
	defer testDone.Store(true)
	p.log = funcr.New(func(prefix, args string) {
		if !testDone.Load() {
			t.Logf("PROXY: %s %s", prefix, args)
		}
	}, funcr.Options{Verbosity: 1})
	if err := p.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}

	client, server := net.Pipe()
	gp := &profile.GameProfile{ID: uuid.New(), Name: username}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.HandleConn(&tunnelProfileConn{Conn: server, gp: gp})
	}()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("HandleConn did not return after the client connection was closed")
		}
	})

	if err := writeTunnelHandshake(client, "gilly-smp.minekube.net", 25565, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client: failed to send handshake: %v", err)
	}
	if err := writeServerLogin(client, username); err != nil {
		t.Fatalf("client: failed to send ServerLogin: %v", err)
	}

	for {
		_ = client.SetReadDeadline(time.Now().Add(10 * time.Second))
		packetID, data, err := readPacket(client)
		if err != nil {
			t.Fatalf("client: read error before login completed: %v", err)
		}
		switch packetID {
		case 0x01: // login-state EncryptionRequest
			t.Fatalf("proxy sent EncryptionRequest into the Connect tunnel; the tunnel-supplied "+
				"GameProfileProvider was masked, so login fell back to online-mode encryption "+
				"(minekube/gate#1081). EncryptionRequest prefix: %x", data[:min(len(data), 16)])
		case 0x02: // login-state LoginSuccess
			r := bytes.NewReader(data)
			gotID, err := util.ReadUUID(r)
			if err != nil {
				t.Fatalf("client: parse LoginSuccess UUID: %v", err)
			}
			gotName, err := util.ReadString(r)
			if err != nil {
				t.Fatalf("client: parse LoginSuccess name: %v", err)
			}
			if gotName != username {
				t.Fatalf("LoginSuccess name = %q, want the tunnel-supplied %q", gotName, username)
			}
			// The tunnel-supplied profile is the identity that completes
			// login, not the unrelated UUID the client put in ServerLogin.
			if gotID != gp.ID {
				t.Fatalf("LoginSuccess UUID = %s, want the tunnel-supplied profile %s", gotID, gp.ID)
			}
			return
		}
	}
}

// writeTunnelHandshake writes a plain (non-Forge) Handshake with login intent to
// the proxy.
func writeTunnelHandshake(w io.Writer, host string, port int, protocolVersion int) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // Handshake packet ID
	pw.VarInt(protocolVersion)
	pw.String(host + "\x00" + "127.0.0.1")
	_ = util.WriteUint16(&payload, uint16(port))
	pw.VarInt(2) // Login intent
	return writeFrame(w, payload.Bytes())
}

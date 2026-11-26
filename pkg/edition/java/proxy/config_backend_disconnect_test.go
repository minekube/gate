package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// A backend that ends the CONFIGURATION phase has to reach the client the way a
// PLAY-phase backend kick does: the client gets a Disconnect carrying the
// backend's reason instead of learning nothing, and the proxy releases the
// player registration instead of keeping the player counted and refusing the
// next attempt with "You are already connected to this server!".
//
// Observed support case (Bark/man900, Gate 0.75.5, Paper 1.21.10 +
// foenichs/Password): the config-phase dialog plugin timed out after its own
// 60 s and disconnected the player in CONFIGURATION. Paper logged
// "lost connection: Server requires correct password." while the client showed
// "Connection Lost: Timed Out" and Gate kept the player online.
func TestConfigPhaseBackendDisconnectReachesClient(t *testing.T) {
	const reason = "Server requires correct password."

	for _, tc := range []struct {
		name string
		// abrupt = the backend closes the socket without a CONFIG Disconnect
		// packet, the shape that ends in backendConfigSessionHandler.Disconnected().
		abrupt        bool
		wantReasonSub string
	}{
		{
			name:          "backend_sends_config_disconnect",
			wantReasonSub: reason,
		},
		{
			// Nothing to relay in this shape, but the client must still be told the
			// join failed instead of hanging until its own keep-alive timeout.
			name:   "backend_closes_without_disconnect",
			abrupt: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			playerName := "Discon" + strings.ToUpper(tc.name[:1])
			playerID := uuid.New()

			backendLn, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("backend listen: %v", err)
			}
			defer backendLn.Close()

			backendDone := make(chan error, 1)
			go runConfigDisconnectingBackend(backendLn, playerID, playerName, reason, tc.abrupt, backendDone)

			p, proxyLn, logs := startConfigDisconnectProxy(t, backendLn.Addr().String())
			defer proxyLn.Close()

			client, err := net.Dial("tcp", proxyLn.Addr().String())
			if err != nil {
				t.Fatalf("client dial: %v", err)
			}
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(15 * time.Second))

			host, port := mustSplitHostPort(t, proxyLn.Addr().String())
			if err := writePlainHandshake(client, host, port, int(ctProtocol.Protocol)); err != nil {
				t.Fatalf("client handshake: %v", err)
			}
			if err := writeModernServerLogin(client, playerName, playerID); err != nil {
				t.Fatalf("client login start: %v", err)
			}

			// LOGIN: the proxy answers with LoginSuccess, then both sides enter CONFIG.
			id, body, err := ctReadPacket(client)
			if err != nil {
				t.Fatalf("client: read login response: %v\nproxy log:\n%s", err, strings.Join(logs.snapshot(), "\n"))
			}
			if id != 0x02 {
				t.Fatalf("client: login response id = 0x%02x, want 0x02 ServerLoginSuccess; body = %x (%q)\nproxy log:\n%s",
					id, body, body, strings.Join(logs.snapshot(), "\n"))
			}
			if err := ctWriteFrame(client, []byte{0x03}); err != nil { // LoginAcknowledged
				t.Fatalf("client LoginAcknowledged: %v", err)
			}

			// CONFIGURATION: the backend ends the join. The client must be told why.
			var disconnectBody []byte
			for disconnectBody == nil {
				id, body, err := ctReadPacket(client)
				if err != nil {
					t.Fatalf("client: the connection ended (%v) without ever receiving a CONFIG Disconnect: "+
						"the client sees a silent close (in the field: \"Connection Lost: Timed Out\") "+
						"instead of the backend's reason\nproxy log:\n%s",
						err, strings.Join(logs.snapshot(), "\n"))
				}
				if id == 0x02 { // CONFIG clientbound Disconnect
					disconnectBody = body
				}
			}
			if body := disconnectBody; tc.wantReasonSub != "" && !bytes.Contains(body, []byte(tc.wantReasonSub)) {
				t.Fatalf("client: CONFIG Disconnect does not carry the backend's reason %q; body = %x",
					tc.wantReasonSub, body)
			}

			// The registration must be released: otherwise the next attempt is refused
			// with "You are already connected to this server!" while the backend reports
			// no players at all.
			waitForPlayerCount(t, p, 0, 5*time.Second, logs)

			select {
			case err := <-backendDone:
				if err != nil {
					t.Fatalf("backend: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("backend never finished\nproxy log:\n%s", strings.Join(logs.snapshot(), "\n"))
			}
		})
	}
}

// runConfigDisconnectingBackend completes LOGIN, then ends the connection in
// CONFIGURATION - with a Disconnect packet, or abruptly when abrupt is set.
func runConfigDisconnectingBackend(
	ln net.Listener,
	playerID uuid.UUID,
	playerName, reason string,
	abrupt bool,
	done chan<- error,
) {
	conn, err := ln.Accept()
	if err != nil {
		done <- err
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	if _, _, err := ctReadPacket(conn); err != nil {
		done <- fmt.Errorf("read handshake: %w", err)
		return
	}
	if _, _, err := ctReadPacket(conn); err != nil {
		done <- fmt.Errorf("read server login: %w", err)
		return
	}
	if err := ctWritePacket(conn, 0x02, &packet.ServerLoginSuccess{
		UUID:     playerID,
		Username: playerName,
	}); err != nil {
		done <- fmt.Errorf("write login success: %w", err)
		return
	}
	if id, _, err := ctReadPacket(conn); err != nil {
		done <- fmt.Errorf("read login acknowledged: %w", err)
		return
	} else if id != 0x03 {
		done <- fmt.Errorf("expected LoginAcknowledged (0x03), got 0x%02x", id)
		return
	}

	if abrupt {
		done <- nil
		return
	}

	// CONFIG clientbound Disconnect (0x02 since 1.20.5), the shape Paper sends when
	// its configuration task chain gives up on the player.
	dc := packet.NewDisconnect(&component.Text{Content: reason}, ctProtocol.Protocol, states.ConfigState)
	if err := cdWritePacket(conn, 0x02, dc, ctProtocol.Protocol); err != nil {
		done <- fmt.Errorf("write config disconnect: %w", err)
		return
	}
	done <- nil
}

// startConfigDisconnectProxy boots a real Gate whose only backend is addr.
func startConfigDisconnectProxy(t *testing.T, backendAddr string) (*Proxy, net.Listener, *logCapture) {
	t.Helper()

	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0"
	cfg.OnlineMode = false
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.Servers = map[string]string{"lobby": backendAddr}
	cfg.Try = []string{"lobby"}

	logs := &logCapture{}
	p, err := New(Options{Config: &cfg, EventMgr: event.New()})
	if err != nil {
		t.Fatalf("proxy New: %v", err)
	}
	p.log = funcr.New(logs.log, funcr.Options{Verbosity: 1})
	if err := p.init(); err != nil {
		t.Fatalf("proxy init: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.HandleConn(conn)
		}
	}()
	return p, ln, logs
}

func waitForPlayerCount(t *testing.T, p *Proxy, want int, timeout time.Duration, logs *logCapture) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if got := p.PlayerCount(); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy player count = %d, want %d after %s: the registration of the player whose "+
				"backend ended the CONFIGURATION phase is leaked\nproxy log:\n%s",
				p.PlayerCount(), want, timeout, strings.Join(logs.snapshot(), "\n"))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// writeModernServerLogin writes a 1.20.2+ LoginStart (name + UUID).
func writeModernServerLogin(w io.Writer, username string, id uuid.UUID) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // Login serverbound ServerLogin
	pw.String(username)
	_ = util.WriteUUID(&payload, id)
	return ctWriteFrame(w, payload.Bytes())
}

// cdWritePacket frames p under id with the id visible to the encoder (a zero id
// would make Disconnect encode its LOGIN-state shape).
func cdWritePacket(w io.Writer, id int, p proto.Packet, protocol proto.Protocol) error {
	var body bytes.Buffer
	if err := p.Encode(&proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  protocol,
		PacketID:  proto.PacketID(id),
	}, &body); err != nil {
		return err
	}
	var frame bytes.Buffer
	util.PanicWriter(&frame).VarInt(id)
	frame.Write(body.Bytes())
	return ctWriteFrame(w, frame.Bytes())
}

func mustSplitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}

var _ = version.Minecraft_1_21_9

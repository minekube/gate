package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Tnze/go-mc/nbt"
	"github.com/go-logr/logr/funcr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// These tests pin the CONFIGURATION-phase relay for an edge-terminated
// (non-passthrough) Connect tunnel session: the shape in which a player arrives
// when the Minekube Connect edge has already authenticated them and hands Gate a
// session proposal carrying a game profile (connectutil's
// tunnelConnWithGameProfile). That path only replaces the proxy's own online-mode
// authentication; the rest of the client connection is the one a direct TCP
// client gets - but it had no wire-level coverage for CONFIG traffic.
//
// Shape being guarded: a Paper config-phase dialog plugin
// (AsyncPlayerConnectionConfigureEvent on a PlayerConfigurationConnection) shows
// a dialog and blocks the backend's configuration task chain until the client's
// serverbound custom click action arrives. If the proxy dropped that packet the
// join would park in CONFIGURATION until the plugin's own timeout - no JoinGame,
// no log line. This test proves the proxy relays the dialog out and the click in,
// for both a CONFIG packet id it knows and one it does not.
var ctProtocol = version.Minecraft_1_21_9 // protocol 773, serves 1.21.9/1.21.10

// ctTunnelConn reproduces the ingress marker stack of a non-passthrough Connect
// tunnel session: a net.Conn that also carries the authenticated game profile and
// the Connect provenance marker, like connectutil's tunnelConnWithGameProfile /
// TunnelSession wrapper.
type ctTunnelConn struct {
	net.Conn
	gp *profile.GameProfile
}

func (c *ctTunnelConn) GameProfile() *profile.GameProfile { return c.gp }
func (c *ctTunnelConn) IsConnectTunnelIngress() bool      { return true }
func (c *ctTunnelConn) Unwrap() net.Conn                  { return c.Conn }

func TestConnectTunnelNonPassthroughConfigDialogClickIsRelayed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		clickID int
	}{
		// CustomClickActionPacket, CONFIG serverbound 0x08 since 1.21.6.
		{name: "known_custom_click_action_id", clickID: 0x08},
		// An id the proxy has no registration for is forwarded as the raw payload
		// (the behaviour #1305 restored), e.g. a packet added by a newer client.
		{name: "unknown_config_id_forwarded_verbatim", clickID: 0x0F},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runConfigDialogRelay(t, byte(tc.clickID))
		})
	}
}

func runConfigDialogRelay(t *testing.T, clickID byte) {
	t.Helper()

	clickData := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03}

	// --- mock backend: login success, CONFIG dialog, then wait for the click ---
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer backendLn.Close()
	backendAddr := backendLn.Addr().String()

	type backendResult struct {
		click []byte
		err   error
	}
	backendCh := make(chan backendResult, 1)

	playerUUID := uuid.New()
	go func() {
		conn, err := backendLn.Accept()
		if err != nil {
			backendCh <- backendResult{err: err}
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

		// Handshake + ServerLogin written by the proxy.
		if _, _, err := ctReadPacket(conn); err != nil {
			backendCh <- backendResult{err: fmt.Errorf("read handshake: %w", err)}
			return
		}
		if _, _, err := ctReadPacket(conn); err != nil {
			backendCh <- backendResult{err: fmt.Errorf("read server login: %w", err)}
			return
		}

		// Login success: the proxy answers LoginAcknowledged and both sides enter
		// CONFIGURATION.
		if err := ctWritePacket(conn, 0x02, &packet.ServerLoginSuccess{
			UUID:     playerUUID,
			Username: "Dial0g",
		}); err != nil {
			backendCh <- backendResult{err: err}
			return
		}
		id, _, err := ctReadPacket(conn)
		if err != nil {
			backendCh <- backendResult{err: fmt.Errorf("read login acknowledged: %w", err)}
			return
		}
		if id != 0x03 { // Login.ServerBound LoginAcknowledged
			backendCh <- backendResult{err: fmt.Errorf("expected LoginAcknowledged (0x03), got 0x%02x", id)}
			return
		}

		// The plugin's dialog: a CONFIG clientbound DialogShow carries only the
		// dialog binary tag.
		if err := ctWritePacket(conn, 0x12, &packet.DialogShow{
			State: states.ConfigState,
			BinaryTag: nbt.RawMessage{
				Type: nbt.TagCompound,
				Data: []byte{nbt.TagEnd},
			},
		}); err != nil {
			backendCh <- backendResult{err: err}
			return
		}

		// The client sends nothing but the click in this test.
		for {
			id, data, err := ctReadPacket(conn)
			if err != nil {
				backendCh <- backendResult{err: fmt.Errorf("waiting for the CONFIG click: %w", err)}
				return
			}
			if byte(id) == clickID {
				backendCh <- backendResult{click: data}
				return
			}
		}
	}()

	// --- real Gate proxy ---
	cfg := config.DefaultConfig
	cfg.OnlineMode = true // the tunnel-supplied profile must replace proxy-side auth
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1
	cfg.Servers = map[string]string{"lobby": backendAddr}
	cfg.Try = []string{"lobby"}

	log := funcr.New(func(prefix, args string) {
		t.Logf("PROXY: %s %s", prefix, args)
	}, funcr.Options{Verbosity: 1})

	// A real event manager is required: for 1.20.2+ the backend dial runs as the
	// "after" callback of the parallel PostLoginEvent, which event.Nop discards -
	// the proxy would then never dial a backend at all.
	p, err := New(Options{Config: &cfg, EventMgr: event.New()})
	if err != nil {
		t.Fatalf("proxy New: %v", err)
	}
	p.log = log
	if err := p.init(); err != nil {
		t.Fatalf("proxy init: %v", err)
	}

	// Accept the client on a real socket, then hand Gate the *wrapped* connection
	// so the non-passthrough tunnel ingress is the one under test.
	clientLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("client listen: %v", err)
	}
	clientConn, err := net.Dial("tcp", clientLn.Addr().String())
	if err != nil {
		clientLn.Close()
		t.Fatalf("client dial: %v", err)
	}
	defer clientConn.Close()
	proxySide, err := clientLn.Accept()
	clientLn.Close()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	_ = clientConn.SetDeadline(time.Now().Add(15 * time.Second))

	go p.HandleConn(&ctTunnelConn{
		Conn: proxySide,
		gp:   &profile.GameProfile{ID: playerUUID, Name: "Dial0g"},
	})

	// --- client: handshake 773 + login start ---
	var hs bytes.Buffer
	hw := util.PanicWriter(&hs)
	hw.VarInt(0x00)
	hw.VarInt(int(ctProtocol.Protocol))
	hw.String("survival.example.net")
	_ = util.WriteUint16(&hs, 25565)
	hw.VarInt(2) // login intent
	if err := ctWriteFrame(clientConn, hs.Bytes()); err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	var ls bytes.Buffer
	lw := util.PanicWriter(&ls)
	lw.VarInt(0x00) // Login.ServerBound ServerLogin
	lw.String("Dial0g")
	_ = util.WriteUUID(&ls, playerUUID)
	if err := ctWriteFrame(clientConn, ls.Bytes()); err != nil {
		t.Fatalf("client login start: %v", err)
	}

	id, _, err := ctReadPacket(clientConn)
	if err != nil {
		t.Fatalf("client: read login response: %v", err)
	}
	switch id {
	case 0x02: // ServerLoginSuccess: the tunnel profile was used
	case 0x01:
		t.Fatalf("proxy sent EncryptionRequest (0x01): the tunnel game profile was not used, " +
			"so this is not the non-passthrough Connect shape")
	default:
		t.Fatalf("client: first login response id = 0x%02x, want 0x02 ServerLoginSuccess", id)
	}

	// LoginAcknowledged: both sides switch to CONFIGURATION.
	if err := ctWriteFrame(clientConn, []byte{0x03}); err != nil {
		t.Fatalf("client LoginAcknowledged: %v", err)
	}

	// The backend's dialog must be forwarded to the client.
	sawDialog := false
	for !sawDialog {
		id, data, err := ctReadPacket(clientConn)
		if err != nil {
			t.Fatalf("client: read while waiting for the CONFIG dialog: %v", err)
		}
		if id != 0x12 { // CONFIG clientbound DialogShow
			continue
		}
		want := []byte{nbt.TagCompound, nbt.TagEnd}
		if !bytes.Equal(data, want) {
			t.Fatalf("client: dialog body = %x, want %x", data, want)
		}
		sawDialog = true
	}

	// Answer the dialog the way a player clicking the button does.
	var click bytes.Buffer
	util.PanicWriter(&click).VarInt(int(clickID))
	click.Write(clickData)
	if err := ctWriteFrame(clientConn, click.Bytes()); err != nil {
		t.Fatalf("client click: %v", err)
	}

	select {
	case got := <-backendCh:
		if got.err != nil {
			t.Fatalf("backend: %v", got.err)
		}
		if !bytes.Equal(got.click, clickData) {
			t.Fatalf("backend click payload = %x, want %x", got.click, clickData)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("backend never received the CONFIG click (id 0x%02x): the join would park in "+
			"CONFIGURATION until the plugin's own timeout", clickID)
	}
}

// ctWritePacket frames p under id using the proxy's own encoder, so the bytes on
// the wire match what a real backend server sends.
func ctWritePacket(w io.Writer, id int, p proto.Packet) error {
	var body bytes.Buffer
	if err := p.Encode(&proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  ctProtocol.Protocol,
	}, &body); err != nil {
		return err
	}
	var frame bytes.Buffer
	util.PanicWriter(&frame).VarInt(id)
	frame.Write(body.Bytes())
	return ctWriteFrame(w, frame.Bytes())
}

func ctWriteFrame(w io.Writer, payload []byte) error {
	var frame bytes.Buffer
	util.PanicWriter(&frame).VarInt(len(payload))
	frame.Write(payload)
	_, err := w.Write(frame.Bytes())
	return err
}

// ctReadPacket reads one uncompressed frame and returns its packet id plus body.
func ctReadPacket(r io.Reader) (int, []byte, error) {
	length, err := ctReadVarInt(r)
	if err != nil {
		return 0, nil, fmt.Errorf("read frame length: %w", err)
	}
	if length <= 0 || length > 1<<20 {
		return 0, nil, fmt.Errorf("invalid frame length %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, fmt.Errorf("read payload: %w", err)
	}
	rd := bytes.NewReader(payload)
	id, err := ctReadVarInt(rd)
	if err != nil {
		return 0, nil, fmt.Errorf("read packet id: %w", err)
	}
	body := make([]byte, rd.Len())
	_, _ = rd.Read(body)
	return id, body, nil
}

func ctReadVarInt(r io.Reader) (int, error) {
	var value int
	for i := 0; i < 5; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		value |= int(b[0]&0x7F) << (7 * i)
		if b[0]&0x80 == 0 {
			return value, nil
		}
	}
	return 0, fmt.Errorf("varint is too big")
}

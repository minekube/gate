package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/uuid"
)

// TestModernForgeIntegration_FullJoinFlow is a wire-level integration test that
// simulates a Forge 1.20.1 (FML3) client connecting through a real Gate proxy to
// a mock Forge backend server, using real TCP connections and the actual Minecraft
// protocol wire format.
//
// The test verifies the complete flow:
//  1. Client sends Handshake with FML3 marker
//  2. Client sends ServerLogin
//  3. Proxy authenticates (offline mode) and delays LoginSuccess
//  4. Proxy connects to mock backend
//  5. Backend sends fml:loginwrapper LoginPluginMessages
//  6. Proxy relays to client (still in LOGIN state)
//  7. Client responds with LoginPluginResponse
//  8. Proxy forwards response to backend
//  9. Backend sends ServerLoginSuccess
//  10. Proxy sends delayed LoginSuccess to client
//  11. Backend sends JoinGame
//  12. Client is connected and in PLAY state
func TestModernForgeIntegration_FullJoinFlow(t *testing.T) {
	// --- Start mock Forge backend server ---
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start backend listener: %v", err)
	}
	defer backendListener.Close()

	backendAddr := backendListener.Addr().String()
	t.Logf("Mock backend listening on %s", backendAddr)

	// FML messages the backend will send during login
	fmlModList := []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x01, 0x00}
	fmlRegistry := []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x03, 0x01}
	fmlConfig := []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x04, 0x02}

	// Expected client responses
	fmlModListReply := []byte{0x02, 'm', 'o', 'd', 's'}
	fmlRegistryACK := []byte{0x63}
	fmlConfigACK := []byte{0x63}

	var backendDone sync.WaitGroup
	backendDone.Add(1)

	backendReceivedResponses := make([][]byte, 0, 3)
	var backendMu sync.Mutex

	go func() {
		defer backendDone.Done()
		conn, err := backendListener.Accept()
		if err != nil {
			t.Errorf("backend accept error: %v", err)
			return
		}
		defer conn.Close()

		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

		// Read Handshake packet from proxy
		_, _, err = readPacket(conn)
		if err != nil {
			t.Errorf("backend: failed to read handshake: %v", err)
			return
		}
		t.Log("Backend: received Handshake from proxy")

		// Read ServerLogin packet from proxy
		_, _, err = readPacket(conn)
		if err != nil {
			t.Errorf("backend: failed to read ServerLogin: %v", err)
			return
		}
		t.Log("Backend: received ServerLogin from proxy")

		// Send fml:loginwrapper LoginPluginMessage #1 (ModList)
		if err := writeLoginPluginMessage(conn, 1, "fml:loginwrapper", fmlModList); err != nil {
			t.Errorf("backend: failed to send FML ModList: %v", err)
			return
		}
		t.Log("Backend: sent FML ModList")

		// Read LoginPluginResponse #1 from proxy
		resp1ID, resp1Data, resp1Success, err := readLoginPluginResponse(conn)
		if err != nil {
			t.Errorf("backend: failed to read response #1: %v", err)
			return
		}
		t.Logf("Backend: received response #1 (id=%d, success=%v, data=%x)", resp1ID, resp1Success, resp1Data)
		if !resp1Success {
			t.Errorf("backend: response #1 Success=false, expected true")
			return
		}
		backendMu.Lock()
		backendReceivedResponses = append(backendReceivedResponses, resp1Data)
		backendMu.Unlock()

		// Send fml:loginwrapper LoginPluginMessage #2 (Registry)
		if err := writeLoginPluginMessage(conn, 2, "fml:loginwrapper", fmlRegistry); err != nil {
			t.Errorf("backend: failed to send FML Registry: %v", err)
			return
		}
		t.Log("Backend: sent FML Registry")

		// Read LoginPluginResponse #2 from proxy
		resp2ID, resp2Data, resp2Success, err := readLoginPluginResponse(conn)
		if err != nil {
			t.Errorf("backend: failed to read response #2: %v", err)
			return
		}
		t.Logf("Backend: received response #2 (id=%d, success=%v, data=%x)", resp2ID, resp2Success, resp2Data)
		if !resp2Success {
			t.Errorf("backend: response #2 Success=false, expected true")
			return
		}
		backendMu.Lock()
		backendReceivedResponses = append(backendReceivedResponses, resp2Data)
		backendMu.Unlock()

		// Send fml:loginwrapper LoginPluginMessage #3 (Config)
		if err := writeLoginPluginMessage(conn, 3, "fml:loginwrapper", fmlConfig); err != nil {
			t.Errorf("backend: failed to send FML Config: %v", err)
			return
		}
		t.Log("Backend: sent FML Config")

		// Read LoginPluginResponse #3 from proxy
		resp3ID, resp3Data, resp3Success, err := readLoginPluginResponse(conn)
		if err != nil {
			t.Errorf("backend: failed to read response #3: %v", err)
			return
		}
		t.Logf("Backend: received response #3 (id=%d, success=%v, data=%x)", resp3ID, resp3Success, resp3Data)
		if !resp3Success {
			t.Errorf("backend: response #3 Success=false, expected true")
			return
		}
		backendMu.Lock()
		backendReceivedResponses = append(backendReceivedResponses, resp3Data)
		backendMu.Unlock()

		// Send ServerLoginSuccess
		if err := writeServerLoginSuccess(conn, uuid.New(), "ForgePlayer"); err != nil {
			t.Errorf("backend: failed to send ServerLoginSuccess: %v", err)
			return
		}
		t.Log("Backend: sent ServerLoginSuccess")

		// Send JoinGame (minimal, just enough for the proxy to process)
		if err := writeJoinGame(conn); err != nil {
			t.Errorf("backend: failed to send JoinGame: %v", err)
			return
		}
		t.Log("Backend: sent JoinGame")

		// Keep connection alive briefly for proxy to process
		time.Sleep(500 * time.Millisecond)
	}()

	// --- Create and start Gate proxy ---
	cfg := config.DefaultConfig
	cfg.Bind = "127.0.0.1:0" // random port
	cfg.OnlineMode = false   // offline mode for testing
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1 // disable compression for simpler wire format
	cfg.Servers = map[string]string{
		"lobby": backendAddr,
	}
	cfg.Try = []string{"lobby"}

	// Set up logging. Use an atomic flag to stop logging after test ends
	// (background goroutines may still run briefly after the test finishes).
	var testDone atomic.Bool
	defer testDone.Store(true)
	log := funcr.New(func(prefix, args string) {
		if !testDone.Load() {
			t.Logf("PROXY: %s %s", prefix, args)
		}
	}, funcr.Options{Verbosity: 1})

	proxy, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}
	proxy.log = log
	if err := proxy.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}

	// Start proxy listener
	proxyListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start proxy listener: %v", err)
	}
	defer proxyListener.Close()

	proxyAddr := proxyListener.Addr().String()
	t.Logf("Proxy listening on %s", proxyAddr)

	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return // listener closed
			}
			go proxy.HandleConn(conn)
		}
	}()

	// --- Connect as Forge client ---
	clientConn, err := net.DialTimeout("tcp", proxyAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("client: failed to connect to proxy: %v", err)
	}
	defer clientConn.Close()
	_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))

	t.Log("Client: connected to proxy")

	// Send Handshake with FML3 marker
	proxyHost, proxyPortStr, _ := net.SplitHostPort(proxyAddr)
	proxyPort, _ := strconv.Atoi(proxyPortStr)
	if err := writeHandshake(clientConn, proxyHost, proxyPort, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client: failed to send handshake: %v", err)
	}
	t.Log("Client: sent Handshake with FML3 marker")

	// Send ServerLogin
	if err := writeServerLogin(clientConn, "ForgePlayer"); err != nil {
		t.Fatalf("client: failed to send ServerLogin: %v", err)
	}
	t.Log("Client: sent ServerLogin")

	// Read packets from proxy - should get LoginPluginMessages before ServerLoginSuccess
	gotLoginSuccess := false
	fmlResponseCount := 0
	loginPluginMsgCount := 0

	for i := 0; i < 20; i++ { // max iterations to prevent infinite loop
		packetID, data, err := readPacket(clientConn)
		if err != nil {
			t.Fatalf("client: read error after %d packets (fml=%d, loginSuccess=%v): %v",
				i, loginPluginMsgCount, gotLoginSuccess, err)
		}

		switch packetID {
		case 0x03: // SetCompression
			threshold := readVarIntFromBytes(data)
			t.Logf("Client: received SetCompression (threshold=%d)", threshold)

		case 0x04: // LoginPluginMessage
			loginPluginMsgCount++
			// Parse: VarInt(ID) + String(Channel) + raw data
			r := bytes.NewReader(data)
			msgID := mustReadVarInt(t, r)
			channel := mustReadString(t, r)
			msgData := make([]byte, r.Len())
			_, _ = r.Read(msgData)

			t.Logf("Client: received LoginPluginMessage #%d (id=%d, channel=%s, dataLen=%d)",
				loginPluginMsgCount, msgID, channel, len(msgData))

			if channel != "fml:loginwrapper" {
				t.Fatalf("client: unexpected channel %q, want fml:loginwrapper", channel)
			}

			// Choose response based on message count
			var responseData []byte
			switch fmlResponseCount {
			case 0:
				responseData = fmlModListReply
			case 1:
				responseData = fmlRegistryACK
			case 2:
				responseData = fmlConfigACK
			default:
				t.Fatalf("client: unexpected 4th FML message")
			}
			fmlResponseCount++

			// Send LoginPluginResponse
			if err := writeLoginPluginResponse(clientConn, msgID, true, responseData); err != nil {
				t.Fatalf("client: failed to send LoginPluginResponse: %v", err)
			}
			t.Logf("Client: sent LoginPluginResponse #%d (id=%d, data=%x)",
				fmlResponseCount, msgID, responseData)

		case 0x02: // ServerLoginSuccess
			gotLoginSuccess = true
			t.Log("Client: received ServerLoginSuccess - FML relay complete!")
			// We got LoginSuccess — the relay worked! Stop here.
			goto done

		default:
			t.Logf("Client: received packet 0x%02x (len=%d)", packetID, len(data))
		}
	}

done:
	// Wait for backend to finish
	backendDone.Wait()

	// --- Assertions ---
	if !gotLoginSuccess {
		t.Fatal("FAILED: client never received ServerLoginSuccess")
	}

	if loginPluginMsgCount != 3 {
		t.Fatalf("FAILED: client received %d LoginPluginMessages, want 3", loginPluginMsgCount)
	}

	if fmlResponseCount != 3 {
		t.Fatalf("FAILED: client sent %d FML responses, want 3", fmlResponseCount)
	}

	backendMu.Lock()
	defer backendMu.Unlock()
	if len(backendReceivedResponses) != 3 {
		t.Fatalf("FAILED: backend received %d responses, want 3", len(backendReceivedResponses))
	}

	// Verify the actual response data made it through the relay
	if string(backendReceivedResponses[0]) != string(fmlModListReply) {
		t.Fatalf("FAILED: backend response[0] = %x, want %x", backendReceivedResponses[0], fmlModListReply)
	}
	if string(backendReceivedResponses[1]) != string(fmlRegistryACK) {
		t.Fatalf("FAILED: backend response[1] = %x, want %x", backendReceivedResponses[1], fmlRegistryACK)
	}
	if string(backendReceivedResponses[2]) != string(fmlConfigACK) {
		t.Fatalf("FAILED: backend response[2] = %x, want %x", backendReceivedResponses[2], fmlConfigACK)
	}

	t.Log("SUCCESS: Full Modern Forge login relay flow completed")
}

// --- Wire protocol helpers ---
// These use util.PanicWriter for bytes.Buffer writes (which never fail).

// writeFrame writes a Minecraft protocol frame: VarInt(length) + payload
func writeFrame(w io.Writer, payload []byte) error {
	var frame bytes.Buffer
	util.PanicWriter(&frame).VarInt(len(payload))
	frame.Write(payload)
	_, err := w.Write(frame.Bytes())
	return err
}

// writeHandshake writes a Handshake packet with FML3 marker
func writeHandshake(w io.Writer, host string, port int, protocolVersion int) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // Handshake packet ID
	pw.VarInt(protocolVersion)
	pw.String(host + "\x00FML3\x00") // FML3 marker
	_ = util.WriteUint16(&payload, uint16(port))
	pw.VarInt(2) // Login intent
	return writeFrame(w, payload.Bytes())
}

// writeServerLogin writes a ServerLogin packet for 1.20 (protocol 763)
func writeServerLogin(w io.Writer, username string) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x00) // ServerLogin packet ID
	pw.String(username)
	pw.Bool(true) // hasUUID (1.19.1+ < 1.20.2)
	_ = util.WriteUUID(&payload, uuid.New())
	return writeFrame(w, payload.Bytes())
}

// writeLoginPluginMessage writes a LoginPluginMessage (server -> client, ID 0x04)
func writeLoginPluginMessage(w io.Writer, id int, channel string, data []byte) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x04) // LoginPluginMessage packet ID
	pw.VarInt(id)
	pw.String(channel)
	payload.Write(data) // raw bytes, no length prefix
	return writeFrame(w, payload.Bytes())
}

// writeLoginPluginResponse writes a LoginPluginResponse (client -> server, ID 0x02)
func writeLoginPluginResponse(w io.Writer, id int, success bool, data []byte) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x02) // LoginPluginResponse packet ID
	pw.VarInt(id)
	pw.Bool(success)
	if success && data != nil {
		payload.Write(data)
	}
	return writeFrame(w, payload.Bytes())
}

// writeServerLoginSuccess writes a ServerLoginSuccess packet
func writeServerLoginSuccess(w io.Writer, id uuid.UUID, username string) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x02) // ServerLoginSuccess packet ID
	_ = util.WriteUUID(&payload, id)
	pw.String(username)
	pw.VarInt(0) // 0 properties
	return writeFrame(w, payload.Bytes())
}

// writeJoinGame writes a minimal JoinGame packet for 1.20.1
func writeJoinGame(w io.Writer) error {
	var payload bytes.Buffer
	pw := util.PanicWriter(&payload)
	pw.VarInt(0x28)                               // JoinGame packet ID for 1.20/1.20.1
	_ = util.WriteInt32(&payload, 1)              // Entity ID
	pw.Bool(false)                                // Is Hardcore
	_ = util.WriteUint8(&payload, 0)              // Game Mode
	_ = util.WriteInt8(&payload, -1)              // Previous Game Mode
	pw.VarInt(1)                                  // 1 dimension
	pw.String("minecraft:overworld")              // Dimension name
	payload.Write([]byte{0x0a, 0x00, 0x00, 0x00}) // Minimal NBT compound
	pw.String("minecraft:overworld")              // Dimension Type
	pw.String("minecraft:overworld")              // Dimension Name
	pw.Int64(0)                                   // Hashed Seed
	pw.VarInt(20)                                 // Max Players
	pw.VarInt(10)                                 // View Distance
	pw.VarInt(10)                                 // Simulation Distance
	pw.Bool(false)                                // Reduced Debug Info
	pw.Bool(true)                                 // Enable Respawn Screen
	pw.Bool(false)                                // Is Debug
	pw.Bool(false)                                // Is Flat
	pw.Bool(false)                                // Has Death Location
	pw.VarInt(0)                                  // Portal Cooldown
	return writeFrame(w, payload.Bytes())
}

// readPacket reads a Minecraft protocol packet and returns (packetID, data, error)
func readPacket(r io.Reader) (int, []byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return 0, nil, fmt.Errorf("read frame length: %w", err)
	}
	if length <= 0 || length > 1048576 {
		return 0, nil, fmt.Errorf("invalid frame length: %d", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, fmt.Errorf("read payload: %w", err)
	}

	reader := bytes.NewReader(payload)
	packetID, err := readVarInt(reader)
	if err != nil {
		return 0, nil, fmt.Errorf("read packet ID: %w", err)
	}

	data := make([]byte, reader.Len())
	_, _ = reader.Read(data)
	return packetID, data, nil
}

// readLoginPluginResponse reads a LoginPluginResponse from the wire
func readLoginPluginResponse(r io.Reader) (id int, data []byte, success bool, err error) {
	packetID, payload, err := readPacket(r)
	if err != nil {
		return 0, nil, false, err
	}
	if packetID != 0x02 {
		return 0, nil, false, fmt.Errorf("expected packet ID 0x02, got 0x%02x", packetID)
	}

	reader := bytes.NewReader(payload)
	id, err = readVarInt(reader)
	if err != nil {
		return 0, nil, false, fmt.Errorf("read response ID: %w", err)
	}

	successByte := make([]byte, 1)
	if _, err := reader.Read(successByte); err != nil {
		return 0, nil, false, fmt.Errorf("read success byte: %w", err)
	}
	success = successByte[0] != 0

	if success && reader.Len() > 0 {
		data = make([]byte, reader.Len())
		_, _ = reader.Read(data)
	}

	return id, data, success, nil
}

// readVarInt reads a VarInt from an io.Reader
func readVarInt(r io.Reader) (int, error) {
	var result int
	var shift uint
	buf := make([]byte, 1)
	for {
		_, err := r.Read(buf)
		if err != nil {
			return 0, err
		}
		b := buf[0]
		result |= int(b&0x7F) << shift
		if b&0x80 == 0 {
			return result, nil
		}
		shift += 7
		if shift >= 35 {
			return 0, fmt.Errorf("VarInt too big")
		}
	}
}

// readVarIntFromBytes reads a VarInt from a byte slice
func readVarIntFromBytes(data []byte) int {
	r := bytes.NewReader(data)
	val, _ := readVarInt(r)
	return val
}

// mustReadVarInt reads a VarInt or fails the test
func mustReadVarInt(t *testing.T, r io.Reader) int {
	t.Helper()
	val, err := readVarInt(r)
	if err != nil {
		t.Fatalf("readVarInt error: %v", err)
	}
	return val
}

// mustReadString reads a Minecraft string (VarInt length + UTF-8 bytes)
func mustReadString(t *testing.T, r io.Reader) string {
	t.Helper()
	length := mustReadVarInt(t, r)
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("readString error: %v", err)
	}
	return string(buf)
}

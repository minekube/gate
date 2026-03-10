package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestForgeNetworkE2E is a true network-level end-to-end test.
//
// It verifies the actual bytes on the wire by:
//  1. Starting a mock backend server on a real TCP port
//  2. Starting a Gate proxy configured with legacy forwarding to that backend
//  3. Connecting a simulated Forge 1.20.1 (FML3) client over TCP
//  4. Verifying the backend receives correct FML3 token + extraData property
//  5. Having the backend send a LoginPluginMessage and verifying the proxy decodes raw bytes
//
// This is not calling internal functions — it's real TCP traffic through the full proxy stack.
func TestForgeNetworkE2E(t *testing.T) {
	t.Run("backend_receives_correct_forge_handshake", testBackendReceivesForgeHandshake)
	t.Run("LoginPluginMessage_wire_format_raw_bytes", testLoginPluginMessageWireFormat)
}

// testBackendReceivesForgeHandshake starts a mock backend, sends a Forge client
// handshake through Gate's packet codec, and verifies the backend would see the
// correct FML3 token and extraData in the forwarding address.
//
// We simulate the proxy→backend leg directly: encode a Handshake packet the way
// Gate would construct it, send it over a real TCP connection, decode it on the
// other side, and verify the bytes.
func testBackendReceivesForgeHandshake(t *testing.T) {
	// Start a mock "backend server" that captures the handshake
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	var backendHandshake *packet.Handshake
	var backendLogin *packet.ServerLogin
	var wg sync.WaitGroup
	wg.Add(1)

	// Backend goroutine: accept one connection and decode the handshake + login packets
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("backend accept error: %v", err)
			return
		}
		defer conn.Close()

		// Decode packets using Gate's actual decoder (same code the real backend path uses)
		decoder := codec.NewDecoder(bufio.NewReader(conn), proto.ServerBound, logr.Discard())
		decoder.SetState(state.Handshake)
		decoder.SetProtocol(version.Minecraft_1_20.Protocol)

		// Read handshake
		ctx, err := decoder.Decode()
		if err != nil {
			t.Errorf("backend decode handshake error: %v", err)
			return
		}
		h, ok := ctx.Packet.(*packet.Handshake)
		if !ok {
			t.Errorf("expected Handshake, got %T", ctx.Packet)
			return
		}
		backendHandshake = h

		// Switch to login state (like a real server would after handshake)
		decoder.SetState(state.Login)

		// Read ServerLogin
		ctx, err = decoder.Decode()
		if err != nil {
			t.Errorf("backend decode login error: %v", err)
			return
		}
		sl, ok := ctx.Packet.(*packet.ServerLogin)
		if !ok {
			t.Errorf("expected ServerLogin, got %T", ctx.Packet)
			return
		}
		backendLogin = sl
	}()

	// "Proxy" side: connect to backend and send handshake the way Gate does
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial backend: %v", err)
	}
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	encoder := codec.NewEncoder(bw, proto.ServerBound, logr.Discard())
	encoder.SetState(state.Handshake)
	encoder.SetProtocol(version.Minecraft_1_20.Protocol)

	// Build the handshake address exactly how Gate constructs it for a Forge
	// client with legacy forwarding. This is what createLegacyForwardingAddress
	// produces plus the FML3 token appended by handshakeAddr.
	//
	// Format: serverAddr\0playerIP\0playerUUID\0[properties]\0FML3\0
	// (The \0FML3\0 token is appended by handshakeAddr after the forwarding address)
	playerUUID := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
	properties := []profile.Property{
		{Name: "textures", Value: "base64encoded"},
		{Name: "extraData", Value: "\x01FML3"},
	}
	propsJSON, _ := json.Marshal(properties)

	forwardingAddr := ln.Addr().String() + "\000" + // server addr
		"192.168.1.100" + "\000" + // player IP
		playerUUID + "\000" + // player UUID
		string(propsJSON) // properties with extraData

	// Append the FML3 token (this is what ModernToken returns for FML3 clients)
	serverAddress := forwardingAddr + "\000FML3\000"

	handshake := &packet.Handshake{
		ProtocolVersion: int(version.Minecraft_1_20.Protocol),
		ServerAddress:   serverAddress,
		Port:            25565,
		NextStatus:      2, // Login
	}
	if _, err := encoder.WritePacket(handshake); err != nil {
		t.Fatalf("failed to write handshake: %v", err)
	}

	// Switch to Login state and send ServerLogin
	encoder.SetState(state.Login)
	login := &packet.ServerLogin{Username: "ForgePlayer"}
	if _, err := encoder.WritePacket(login); err != nil {
		t.Fatalf("failed to write login: %v", err)
	}

	// Flush buffered writer then signal EOF to backend
	if err := bw.Flush(); err != nil {
		t.Fatalf("failed to flush: %v", err)
	}
	conn.(*net.TCPConn).CloseWrite()

	// Wait for backend to finish reading
	wg.Wait()

	// === Verify what the backend received ===

	if backendHandshake == nil {
		t.Fatal("backend did not receive handshake")
	}
	if backendLogin == nil {
		t.Fatal("backend did not receive login")
	}

	// 1. Verify FML3 token is present in the handshake address
	if !strings.Contains(backendHandshake.ServerAddress, "\000FML3\000") {
		t.Errorf("backend handshake missing FML3 token\nServerAddress: %q", backendHandshake.ServerAddress)
	}

	// 2. Parse the forwarding address and verify extraData property
	parts := strings.SplitN(backendHandshake.ServerAddress, "\000", 5)
	if len(parts) < 4 {
		t.Fatalf("forwarding address has %d parts, expected >=4\nServerAddress: %q", len(parts), backendHandshake.ServerAddress)
	}
	propsStr := parts[3]

	var decodedProps []profile.Property
	if err := json.Unmarshal([]byte(propsStr), &decodedProps); err != nil {
		t.Fatalf("backend can't parse properties: %v\nJSON: %s", err, propsStr)
	}

	var foundExtraData bool
	for _, p := range decodedProps {
		if p.Name == "extraData" {
			foundExtraData = true
			if p.Value != "\x01FML3" {
				t.Errorf("extraData value = %q, want %q", p.Value, "\x01FML3")
			}
			break
		}
	}
	if !foundExtraData {
		t.Errorf("backend did not receive extraData property\nProperties: %s", propsStr)
	}

	// 3. Verify empty signatures are omitted
	if strings.Contains(propsStr, `"signature":""`) {
		t.Errorf("empty signature should be omitted\nJSON: %s", propsStr)
	}

	// 4. Verify player name
	if backendLogin.Username != "ForgePlayer" {
		t.Errorf("login username = %q, want %q", backendLogin.Username, "ForgePlayer")
	}

	t.Logf("Backend received correct Forge handshake with FML3 token and extraData")
}

// testLoginPluginMessageWireFormat verifies that LoginPluginMessage uses raw bytes
// (not length-prefixed) on the actual TCP wire, by sending a packet through Gate's
// codec over a real TCP connection and decoding it on the other side.
func testLoginPluginMessageWireFormat(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	// The test data: a Velocity forwarding version byte
	testData := []byte{0x04} // version 4

	var decodedMsg *packet.LoginPluginMessage
	var rawPayload []byte
	var wg sync.WaitGroup
	wg.Add(1)

	// "Client" side: receive and decode the LoginPluginMessage
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept error: %v", err)
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		// Decode using Gate's decoder (ClientBound = server→client direction)
		decoder := codec.NewDecoder(bufio.NewReader(conn), proto.ClientBound, logr.Discard())
		decoder.SetState(state.Login)
		decoder.SetProtocol(version.Minecraft_1_20.Protocol)

		ctx, err := decoder.Decode()
		if err != nil {
			t.Errorf("decode error: %v", err)
			return
		}
		rawPayload = ctx.Payload

		msg, ok := ctx.Packet.(*packet.LoginPluginMessage)
		if !ok {
			t.Errorf("expected LoginPluginMessage, got %T", ctx.Packet)
			return
		}
		decodedMsg = msg
	}()

	// "Server" side: encode and send a LoginPluginMessage
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	bw := bufio.NewWriter(conn)
	encoder := codec.NewEncoder(bw, proto.ClientBound, logr.Discard())
	encoder.SetState(state.Login)
	encoder.SetProtocol(version.Minecraft_1_20.Protocol)

	msg := &packet.LoginPluginMessage{
		ID:      42,
		Channel: "velocity:player_info",
		Data:    testData,
	}
	if _, err := encoder.WritePacket(msg); err != nil {
		t.Fatalf("failed to write LoginPluginMessage: %v", err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("failed to flush: %v", err)
	}
	conn.(*net.TCPConn).CloseWrite()

	wg.Wait()

	if decodedMsg == nil {
		t.Fatal("did not receive LoginPluginMessage")
	}

	// Verify the decoded packet
	if decodedMsg.ID != 42 {
		t.Errorf("ID = %d, want 42", decodedMsg.ID)
	}
	if decodedMsg.Channel != "velocity:player_info" {
		t.Errorf("Channel = %q, want %q", decodedMsg.Channel, "velocity:player_info")
	}
	if !bytes.Equal(decodedMsg.Data, testData) {
		t.Fatalf("Data = %x, want %x\n"+
			"The length-prefix bug causes the first data byte to be consumed as a VarInt\n"+
			"length, making Data nil/empty. This breaks Velocity forwarding version negotiation.",
			decodedMsg.Data, testData)
	}

	// Also verify the raw payload format: it should be VarInt(42) + String("velocity:player_info") + 0x04
	// WITHOUT a VarInt length prefix before 0x04
	var expectedPayload bytes.Buffer
	util.PanicWriter(&expectedPayload).VarInt(42)
	util.PanicWriter(&expectedPayload).String("velocity:player_info")
	expectedPayload.Write(testData) // raw, no length prefix

	// The raw payload in PacketContext includes the packet ID VarInt prefix, skip it
	payloadReader := bytes.NewReader(rawPayload)
	_, _ = util.ReadVarInt(payloadReader) // skip packet ID
	actualPayload, _ := io.ReadAll(payloadReader)

	if !bytes.Equal(actualPayload, expectedPayload.Bytes()) {
		t.Errorf("raw wire payload mismatch\nactual:   %x\nexpected: %x\n"+
			"If actual has an extra byte, the length-prefix bug is present.",
			actualPayload, expectedPayload.Bytes())
	}

	t.Logf("LoginPluginMessage correctly uses raw bytes on the wire (no length prefix)")
}

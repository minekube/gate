package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"go.minekube.com/gate/pkg/edition/java/forge/modernforge"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/netutil"
)

// TestForgeE2E_FML3ClientToBackend is an end-to-end test simulating a Forge 1.20.1
// (FML3) client connecting through Gate to a backend server with BungeeCord legacy
// forwarding. It verifies the full flow:
//
//  1. Client sends Handshake with FML3 marker → Gate detects ModernForge
//  2. Gate constructs backend handshake address → preserves FML3 token
//  3. Gate constructs legacy forwarding address → includes BungeeForge extraData
//  4. Backend sends LoginPluginMessage → Gate decodes raw bytes correctly
//
// This test catches the bugs from PR #613 where:
//   - FML2/FML3 clients were classified as Vanilla (no marker detection)
//   - ModernToken returned \0FORGE instead of \0FML3\0
//   - Legacy forwarding didn't include extraData for BungeeForge
//   - LoginPluginMessage data was decoded with a spurious length prefix
func TestForgeE2E_FML3ClientToBackend(t *testing.T) {
	// === Step 1: Forge client sends Handshake with FML3 marker ===
	//
	// A real Forge 1.20.1 client sends a handshake like:
	//   ServerAddress: "play.example.com\0FML3\0"
	//   ProtocolVersion: 763 (1.20/1.20.1)
	clientHandshake := &packet.Handshake{
		ServerAddress:   "play.example.com\000FML3\000",
		ProtocolVersion: int(version.Minecraft_1_20.Protocol),
		Port:            25565,
		NextStatus:      2, // Login
	}

	// Gate must detect this as ModernForge (not Vanilla!)
	connType := handshakeConnectionType(clientHandshake)
	if connType != phase.ModernForge {
		t.Fatalf("Step 1 FAILED: FML3 client detected as %T(%p), want ModernForge(%p)\n"+
			"This means Forge 1.20.1 clients are treated as vanilla and the backend\n"+
			"will reject with: 'Channels [...] rejected vanilla connections'",
			connType, connType, phase.ModernForge)
	}
	t.Log("Step 1 PASSED: FML3 client correctly detected as ModernForge")

	// === Step 2: Gate builds backend handshake address with FML3 token ===
	//
	// When connecting to the backend, Gate must append the correct Forge
	// token to the handshake address. For FML3, it must be "\0FML3\0".
	backendToken := modernforge.ModernToken(clientHandshake.ServerAddress)
	if backendToken != "\000FML3\000" {
		t.Fatalf("Step 2 FAILED: ModernToken returned %q, want %q\n"+
			"Backend won't see the Forge marker, breaking Forge handshake.",
			backendToken, "\000FML3\000")
	}
	t.Log("Step 2 PASSED: FML3 token correctly preserved for backend handshake")

	// === Step 3: Gate includes BungeeForge extraData in legacy forwarding ===
	//
	// With BungeeCord legacy forwarding, the handshake address contains:
	//   server_addr\0player_ip\0player_uuid\0[{properties}]
	// BungeeForge expects a property: {"name":"extraData","value":"\x01FML3"}
	player := &connectedPlayer{
		MinecraftConn: &testMinecraftConn{connType: phase.ModernForge},
		log:           logr.Discard(),
		profile: &profile.GameProfile{
			Properties: []profile.Property{
				{Name: "textures", Value: "base64data"},
			},
		},
		virtualHost: netutil.NewAddr("play.example.com\000FML3\000:25565", "tcp"),
	}

	backendAddr := mustParseAddr("localhost:25566")
	serverConn := &serverConnection{
		server: newRegisteredServer(NewServerInfo("forge-backend", backendAddr)),
		player: player,
		log:    logr.Discard(),
	}

	forwardAddr := serverConn.createLegacyForwardingAddress()

	// Parse the forwarding address: parts are separated by \0
	parts := strings.SplitN(forwardAddr, "\000", 4)
	if len(parts) != 4 {
		t.Fatalf("Step 3 FAILED: forwarding address has %d parts, expected 4", len(parts))
	}
	propertiesJSON := parts[3]

	var properties []profile.Property
	if err := json.Unmarshal([]byte(propertiesJSON), &properties); err != nil {
		t.Fatalf("Step 3 FAILED: can't parse properties JSON: %v\nJSON: %s", err, propertiesJSON)
	}

	// Find the extraData property
	var extraData *profile.Property
	for i := range properties {
		if properties[i].Name == "extraData" {
			extraData = &properties[i]
			break
		}
	}
	if extraData == nil {
		t.Fatalf("Step 3 FAILED: no 'extraData' property in forwarding address\n"+
			"Properties: %s\n"+
			"BungeeForge on the backend won't detect this as a Forge client,\n"+
			"causing: 'Channels [...] rejected vanilla connections'",
			propertiesJSON)
	}
	if extraData.Value != "\x01FML3" {
		t.Fatalf("Step 3 FAILED: extraData value = %q, want %q",
			extraData.Value, "\x01FML3")
	}
	// Verify empty signatures are omitted (clean JSON)
	if strings.Contains(propertiesJSON, `"signature":""`) {
		t.Fatalf("Step 3 FAILED: empty signature should be omitted from JSON\nJSON: %s", propertiesJSON)
	}
	t.Log("Step 3 PASSED: legacy forwarding includes BungeeForge extraData property")

	// === Step 4: Backend sends LoginPluginMessage, Gate decodes raw bytes ===
	//
	// The Minecraft protocol says LoginPluginMessage.Data is the remaining
	// bytes in the packet (NOT length-prefixed). When a backend server sends
	// a Velocity forwarding request, the data is a single byte [version].
	// With the old buggy codec, the VarInt length prefix consumed the version
	// byte, making the data nil and silently falling back to version 1.
	t.Run("LoginPluginMessage_velocity_forwarding", func(t *testing.T) {
		testLoginPluginMessageRawDecode(t,
			42, "velocity:player_info", []byte{0x04},
			"Velocity forwarding version should be 0x04, not swallowed by length prefix")
	})

	t.Run("LoginPluginMessage_forge_handshake", func(t *testing.T) {
		// Forge sends fml:loginwrapper with binary handshake data
		forgeData := []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x01, 0x00}
		testLoginPluginMessageRawDecode(t,
			0, "fml:loginwrapper", forgeData,
			"Forge handshake data must be decoded as raw bytes")
	})
}

// testLoginPluginMessageRawDecode builds a LoginPluginMessage as raw protocol
// bytes (how a real server sends it) and verifies Gate decodes it correctly.
func testLoginPluginMessageRawDecode(t *testing.T, id int, channel string, data []byte, msg string) {
	t.Helper()

	// Build raw protocol bytes as a real server would send them:
	// VarInt(ID) + String(Channel) + raw data bytes (NO length prefix)
	var rawPacket bytes.Buffer
	util.PanicWriter(&rawPacket).VarInt(id)
	util.PanicWriter(&rawPacket).String(channel)
	rawPacket.Write(data) // raw bytes, no length prefix

	// Decode using Gate's LoginPluginMessage decoder
	decoded := &packet.LoginPluginMessage{}
	ctx := &proto.PacketContext{
		Direction: proto.ClientBound,
		Protocol:  version.Minecraft_1_20.Protocol,
	}
	if err := decoded.Decode(ctx, &rawPacket); err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if decoded.ID != id {
		t.Errorf("ID = %d, want %d", decoded.ID, id)
	}
	if decoded.Channel != channel {
		t.Errorf("Channel = %q, want %q", decoded.Channel, channel)
	}
	if !bytes.Equal(decoded.Data, data) {
		t.Fatalf("FAILED: %s\nData = %x, want %x\n"+
			"If Data is nil or wrong, the length-prefix bug is present:\n"+
			"the first byte of data was consumed as a VarInt length prefix.",
			msg, decoded.Data, data)
	}
}

// TestForgeE2E_FML2ClientDetection tests that Forge 1.13-1.17 (FML2) clients
// are also correctly detected and forwarded.
func TestForgeE2E_FML2ClientDetection(t *testing.T) {
	handshake := &packet.Handshake{
		ServerAddress:   "server.example.com\000FML2\000",
		ProtocolVersion: int(version.Minecraft_1_16_4.Protocol),
	}

	connType := handshakeConnectionType(handshake)
	if connType != phase.ModernForge {
		t.Fatalf("FML2 (1.16.4) client detected as %T(%p), want ModernForge(%p)",
			connType, connType, phase.ModernForge)
	}

	token := modernforge.ModernToken(handshake.ServerAddress)
	if token != "\000FML2\000" {
		t.Fatalf("ModernToken returned %q, want %q", token, "\000FML2\000")
	}
}

// TestForgeE2E_BungeeGuardForwarding verifies that BungeeGuard forwarding also
// includes the extraData property for Forge clients.
func TestForgeE2E_BungeeGuardForwarding(t *testing.T) {
	player := &connectedPlayer{
		MinecraftConn: &testMinecraftConn{connType: phase.ModernForge},
		log:           logr.Discard(),
		profile: &profile.GameProfile{
			Properties: []profile.Property{},
		},
		virtualHost: netutil.NewAddr("play.example.com\000FML3\000:25565", "tcp"),
	}

	serverConn := &serverConnection{
		server: newRegisteredServer(NewServerInfo("forge-backend", mustParseAddr("localhost:25566"))),
		player: player,
		log:    logr.Discard(),
	}

	forwardAddr := serverConn.createBungeeGuardForwardingAddress("my-secret-token")
	parts := strings.SplitN(forwardAddr, "\000", 4)
	if len(parts) != 4 {
		t.Fatalf("forwarding address has %d parts, expected 4", len(parts))
	}

	var properties []profile.Property
	if err := json.Unmarshal([]byte(parts[3]), &properties); err != nil {
		t.Fatalf("can't parse properties JSON: %v", err)
	}

	var hasExtraData, hasBungeeGuard bool
	for _, p := range properties {
		if p.Name == "extraData" && p.Value == "\x01FML3" {
			hasExtraData = true
		}
		if p.Name == "bungeeguard-token" && p.Value == "my-secret-token" {
			hasBungeeGuard = true
		}
	}

	if !hasExtraData {
		t.Error("BungeeGuard forwarding missing extraData property for Forge client")
	}
	if !hasBungeeGuard {
		t.Error("BungeeGuard forwarding missing bungeeguard-token property")
	}
}

// TestForgeE2E_VanillaClientUnchanged verifies that vanilla clients are not
// affected by the Forge changes (no extraData, no FML token).
func TestForgeE2E_VanillaClientUnchanged(t *testing.T) {
	handshake := &packet.Handshake{
		ServerAddress:   "server.example.com",
		ProtocolVersion: int(version.Minecraft_1_20.Protocol),
	}

	connType := handshakeConnectionType(handshake)
	if connType != phase.Vanilla {
		t.Fatalf("vanilla client detected as %T(%p), want Vanilla(%p)",
			connType, connType, phase.Vanilla)
	}

	player := &connectedPlayer{
		MinecraftConn: &testMinecraftConn{connType: phase.Vanilla},
		log:           logr.Discard(),
		profile:       &profile.GameProfile{},
		virtualHost:   netutil.NewAddr("server.example.com:25565", "tcp"),
	}

	serverConn := &serverConnection{
		server: newRegisteredServer(NewServerInfo("vanilla-backend", mustParseAddr("localhost:25566"))),
		player: player,
		log:    logr.Discard(),
	}

	forwardAddr := serverConn.createLegacyForwardingAddress()
	parts := strings.SplitN(forwardAddr, "\000", 4)
	if len(parts) != 4 {
		t.Fatalf("forwarding address has %d parts, expected 4", len(parts))
	}

	// Vanilla client should NOT have extraData
	if strings.Contains(parts[3], "extraData") {
		t.Fatalf("vanilla client should not have extraData in forwarding: %s", parts[3])
	}
}

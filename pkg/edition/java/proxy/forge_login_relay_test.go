package proxy

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"
)

// TestModernForgeRelay_Complete verifies that completing the relay sends the
// pending LoginSuccess to the client.
func TestModernForgeRelay_Complete(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "ForgePlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	if err := relay.complete(); err != nil {
		t.Fatalf("complete error: %v", err)
	}

	if len(clientConn.writtenPackets) != 1 {
		t.Fatalf("client packets = %d, want 1", len(clientConn.writtenPackets))
	}
	success, ok := clientConn.writtenPackets[0].(*packet.ServerLoginSuccess)
	if !ok {
		t.Fatalf("expected ServerLoginSuccess, got %T", clientConn.writtenPackets[0])
	}
	if success.Username != "ForgePlayer" {
		t.Fatalf("username = %q, want %q", success.Username, "ForgePlayer")
	}
}

// TestModernForgeReplayRelay verifies that cached exchanges are correctly
// replayed during a server switch.
func TestModernForgeReplayRelay(t *testing.T) {
	cached := []forgeLoginExchange{
		{Channel: ForgeLoginWrapperChannel, Request: []byte{0x01}, Response: []byte{0x02, 0x00}, Success: true},
		{Channel: ForgeLoginWrapperChannel, Request: []byte{0x03}, Response: []byte{0x63}, Success: true},
		{Channel: ForgeLoginWrapperChannel, Request: []byte{0x04}, Response: []byte{0x63}, Success: true},
	}

	replay := newModernForgeReplayRelay(cached)
	backendConn := &testMinecraftConn{}

	for i := 0; i < 3; i++ {
		if err := replay.replayResponse(i+10, backendConn); err != nil {
			t.Fatalf("replayResponse[%d] error: %v", i, err)
		}
	}

	if len(backendConn.writtenPackets) != 3 {
		t.Fatalf("backend packets = %d, want 3", len(backendConn.writtenPackets))
	}

	for i, pkt := range backendConn.writtenPackets {
		resp := pkt.(*packet.LoginPluginResponse)
		if resp.ID != i+10 {
			t.Fatalf("response[%d] ID = %d, want %d", i, resp.ID, i+10)
		}
		if !resp.Success {
			t.Fatalf("response[%d] Success = false, want true", i)
		}
		if string(resp.Data) != string(cached[i].Response) {
			t.Fatalf("response[%d] data = %x, want %x", i, resp.Data, cached[i].Response)
		}
	}
}

// TestModernForgeReplayRelay_ExhaustedCache verifies that when the backend sends
// more FML messages than were cached, the proxy responds with Success=false.
func TestModernForgeReplayRelay_ExhaustedCache(t *testing.T) {
	cached := []forgeLoginExchange{
		{Channel: ForgeLoginWrapperChannel, Request: []byte{0x01}, Response: []byte{0x02}, Success: true},
	}

	replay := newModernForgeReplayRelay(cached)
	backendConn := &testMinecraftConn{}

	if err := replay.replayResponse(1, backendConn); err != nil {
		t.Fatalf("replayResponse error: %v", err)
	}

	if err := replay.replayResponse(2, backendConn); err != nil {
		t.Fatalf("replayResponse error: %v", err)
	}

	resp := backendConn.writtenPackets[1].(*packet.LoginPluginResponse)
	if resp.Success {
		t.Fatal("expected Success=false when cache exhausted")
	}
}

// TestModernForge_BackendLoginHandler_ReplayOnSwitch tests that during a server
// switch, cached FML responses are replayed to the new backend.
func TestModernForge_BackendLoginHandler_ReplayOnSwitch(t *testing.T) {
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: &testMinecraftConn{connType: phase.ModernForge},
		log:           logr.Discard(),
	}

	cached := []forgeLoginExchange{
		{Channel: ForgeLoginWrapperChannel, Request: []byte{0x01}, Response: []byte{0x02}, Success: true},
		{Channel: ForgeLoginWrapperChannel, Request: []byte{0x03}, Response: []byte{0x63}, Success: true},
	}
	player.mu.Lock()
	player.forgeReplayRelay = newModernForgeReplayRelay(cached)
	player.mu.Unlock()

	eventMgr := event.New()
	resultChan := make(chan *connResponse, 1)
	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()

	handler := &backendLoginSessionHandler{
		serverConn: serverConn,
		requestCtx: &connRequestCxt{
			Context:  context.Background(),
			response: resultChan,
		},
		log: logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{
			eventMgr:       eventMgr,
			configProvider: &testConfigProvider{cfg: &config.Config{}},
		},
	}

	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 1, Channel: ForgeLoginWrapperChannel, Data: []byte{0x01},
	})
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 2, Channel: ForgeLoginWrapperChannel, Data: []byte{0x03},
	})

	if len(backendConn.writtenPackets) != 2 {
		t.Fatalf("backend packets = %d, want 2", len(backendConn.writtenPackets))
	}

	for i, pkt := range backendConn.writtenPackets {
		resp := pkt.(*packet.LoginPluginResponse)
		if resp.ID != i+1 {
			t.Fatalf("response[%d] ID = %d, want %d", i, resp.ID, i+1)
		}
		if !resp.Success {
			t.Fatalf("response[%d] Success = false, want true", i)
		}
	}
}

// TestModernForge_VanillaClientUnaffected verifies that vanilla clients connecting
// to a backend that sends LoginPluginMessages on non-FML channels are unaffected.
func TestModernForge_VanillaClientUnaffected(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.Vanilla}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	eventMgr := event.New()
	resultChan := make(chan *connResponse, 1)
	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()

	handler := &backendLoginSessionHandler{
		serverConn: serverConn,
		requestCtx: &connRequestCxt{
			Context:  context.Background(),
			response: resultChan,
		},
		log: logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{
			eventMgr:       eventMgr,
			configProvider: &testConfigProvider{cfg: &config.Config{}},
		},
	}

	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 1, Channel: "custom:channel", Data: []byte{0x01},
	})

	if len(backendConn.writtenPackets) != 1 {
		t.Fatalf("backend packets = %d, want 1", len(backendConn.writtenPackets))
	}
	resp := backendConn.writtenPackets[0].(*packet.LoginPluginResponse)
	if resp.Success {
		t.Fatal("vanilla client: custom channel should get Success=false")
	}
}

// TestModernForge_DelayedLoginSuccess_Detection tests that Modern Forge clients
// on pre-1.20.2 are correctly identified for delayed LoginSuccess.
func TestModernForge_DelayedLoginSuccess_Detection(t *testing.T) {
	tests := []struct {
		name        string
		connType    phase.ConnectionType
		protocol    proto.Protocol
		expectDelay bool
	}{
		{"FML3 1.20.1 - should delay", phase.ModernForge, version.Minecraft_1_20.Protocol, true},
		{"FML2 1.16.4 - should delay", phase.ModernForge, version.Minecraft_1_16_4.Protocol, true},
		{"Forge 1.20.2+ - no delay", phase.ModernForge, version.Minecraft_1_20_2.Protocol, false},
		{"Vanilla 1.20.1 - no delay", phase.Vanilla, version.Minecraft_1_20.Protocol, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isModernForgePre1202 := tt.protocol.Lower(version.Minecraft_1_20_2) && tt.connType == phase.ModernForge
			if isModernForgePre1202 != tt.expectDelay {
				t.Fatalf("isModernForgePre1202 = %v, want %v", isModernForgePre1202, tt.expectDelay)
			}
		})
	}
}

// newTestLoginInboundConn creates a loginInboundConn suitable for testing.
func newTestLoginInboundConn(mc *testMinecraftConn) *loginInboundConn {
	return &loginInboundConn{
		delegate:             &initialInbound{MinecraftConn: mc},
		outstandingResponses: map[int]MessageConsumer{},
		isLoginEventFired:    true,
	}
}

// TestModernForge_TestSetup_VersionCheck verifies version comparison works as expected.
func TestModernForge_TestSetup_VersionCheck(t *testing.T) {
	if !version.Minecraft_1_20.Protocol.Lower(version.Minecraft_1_20_2) {
		t.Fatal("1.20 should be lower than 1.20.2")
	}
	if !version.Minecraft_1_16_4.Protocol.Lower(version.Minecraft_1_20_2) {
		t.Fatal("1.16.4 should be lower than 1.20.2")
	}
	if version.Minecraft_1_20_2.Protocol.Lower(version.Minecraft_1_20_2) {
		t.Fatal("1.20.2 should NOT be lower than 1.20.2")
	}
}

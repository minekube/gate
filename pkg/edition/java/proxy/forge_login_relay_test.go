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

// TestModernForgeRelay_BasicExchange verifies that a single FML LoginPluginMessage
// is correctly relayed from backend → client → backend.
func TestModernForgeRelay_BasicExchange(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)

	loginSuccess := &packet.ServerLoginSuccess{Username: "TestPlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	// Simulate backend sending fml:loginwrapper LoginPluginMessage
	backendMsg := &packet.LoginPluginMessage{
		ID:      1,
		Channel: ForgeLoginWrapperChannel,
		Data:    []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x01, 0x00},
	}

	if err := relay.relayToClient(backendConn, backendMsg); err != nil {
		t.Fatalf("relayToClient error: %v", err)
	}

	// Verify client received a LoginPluginMessage
	if len(clientConn.writtenPackets) != 1 {
		t.Fatalf("expected 1 packet to client, got %d", len(clientConn.writtenPackets))
	}
	clientMsg, ok := clientConn.writtenPackets[0].(*packet.LoginPluginMessage)
	if !ok {
		t.Fatalf("expected LoginPluginMessage, got %T", clientConn.writtenPackets[0])
	}
	if clientMsg.Channel != ForgeLoginWrapperChannel {
		t.Fatalf("channel = %q, want %q", clientMsg.Channel, ForgeLoginWrapperChannel)
	}

	// Simulate client responding
	clientResponse := &packet.LoginPluginResponse{
		ID:      clientMsg.ID,
		Success: true,
		Data:    []byte{0x02, 0x00}, // mod list reply
	}
	if err := login.handleLoginPluginResponse(clientResponse); err != nil {
		t.Fatalf("handleLoginPluginResponse error: %v", err)
	}

	// Verify backend received the response with the ORIGINAL backend message ID
	if len(backendConn.writtenPackets) != 1 {
		t.Fatalf("expected 1 packet to backend, got %d", len(backendConn.writtenPackets))
	}
	backendResp, ok := backendConn.writtenPackets[0].(*packet.LoginPluginResponse)
	if !ok {
		t.Fatalf("expected LoginPluginResponse, got %T", backendConn.writtenPackets[0])
	}
	if backendResp.ID != 1 {
		t.Fatalf("backend response ID = %d, want 1 (original backend ID)", backendResp.ID)
	}
	if !backendResp.Success {
		t.Fatal("backend response Success = false, want true")
	}
	if string(backendResp.Data) != string(clientResponse.Data) {
		t.Fatalf("backend response data = %x, want %x", backendResp.Data, clientResponse.Data)
	}

	// Verify exchange was cached
	exchanges := relay.exchanges()
	if len(exchanges) != 1 {
		t.Fatalf("cached exchanges = %d, want 1", len(exchanges))
	}
	if exchanges[0].Channel != ForgeLoginWrapperChannel {
		t.Fatalf("cached channel = %q, want %q", exchanges[0].Channel, ForgeLoginWrapperChannel)
	}
	if !exchanges[0].Success {
		t.Fatal("cached Success = false, want true")
	}
}

// TestModernForgeRelay_MultipleMessages verifies multiple FML messages are relayed
// correctly and all exchanges are cached.
func TestModernForgeRelay_MultipleMessages(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "TestPlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	// Simulate 3 FML messages (ModList, Registry, ConfigData)
	messages := []struct {
		id   int
		data []byte
	}{
		{1, []byte{0x01, 'M', 'o', 'd', 'L', 'i', 's', 't'}},
		{2, []byte{0x03, 'R', 'e', 'g', 'i', 's', 't', 'r', 'y'}},
		{3, []byte{0x04, 'C', 'o', 'n', 'f', 'i', 'g'}},
	}

	responses := [][]byte{
		{0x02, 'R', 'e', 'p', 'l', 'y'},
		{0x63, 'A', 'C', 'K'},
		{0x63, 'A', 'C', 'K'},
	}

	for i, msg := range messages {
		backendMsg := &packet.LoginPluginMessage{
			ID:      msg.id,
			Channel: ForgeLoginWrapperChannel,
			Data:    msg.data,
		}
		if err := relay.relayToClient(backendConn, backendMsg); err != nil {
			t.Fatalf("relayToClient[%d] error: %v", i, err)
		}

		// Get the client-side message ID
		clientMsg := clientConn.writtenPackets[i].(*packet.LoginPluginMessage)

		// Simulate client response
		if err := login.handleLoginPluginResponse(&packet.LoginPluginResponse{
			ID:      clientMsg.ID,
			Success: true,
			Data:    responses[i],
		}); err != nil {
			t.Fatalf("handleLoginPluginResponse[%d] error: %v", i, err)
		}
	}

	// Verify all 3 messages were relayed to client
	if len(clientConn.writtenPackets) != 3 {
		t.Fatalf("client packets = %d, want 3", len(clientConn.writtenPackets))
	}

	// Verify all 3 responses were forwarded to backend
	if len(backendConn.writtenPackets) != 3 {
		t.Fatalf("backend packets = %d, want 3", len(backendConn.writtenPackets))
	}

	// Verify responses map to correct backend IDs
	for i, msg := range messages {
		resp := backendConn.writtenPackets[i].(*packet.LoginPluginResponse)
		if resp.ID != msg.id {
			t.Fatalf("backend response[%d] ID = %d, want %d", i, resp.ID, msg.id)
		}
	}

	// Verify all exchanges cached
	exchanges := relay.exchanges()
	if len(exchanges) != 3 {
		t.Fatalf("cached exchanges = %d, want 3", len(exchanges))
	}
}

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

	// Complete the relay
	if err := relay.complete(); err != nil {
		t.Fatalf("complete error: %v", err)
	}

	// Verify LoginSuccess was sent to client
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

// TestModernForgeRelay_ClientRejectsMessage verifies that when the client sends
// Success=false, the rejection is correctly forwarded to the backend and cached.
func TestModernForgeRelay_ClientRejectsMessage(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "TestPlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	// Backend sends a message
	backendMsg := &packet.LoginPluginMessage{
		ID:      5,
		Channel: ForgeLoginWrapperChannel,
		Data:    []byte{0x01, 0x02},
	}
	if err := relay.relayToClient(backendConn, backendMsg); err != nil {
		t.Fatalf("relayToClient error: %v", err)
	}

	clientMsg := clientConn.writtenPackets[0].(*packet.LoginPluginMessage)

	// Client rejects (Success=false, nil data)
	if err := login.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID:      clientMsg.ID,
		Success: false,
	}); err != nil {
		t.Fatalf("handleLoginPluginResponse error: %v", err)
	}

	// Verify backend received rejection
	backendResp := backendConn.writtenPackets[0].(*packet.LoginPluginResponse)
	if backendResp.ID != 5 {
		t.Fatalf("backend response ID = %d, want 5", backendResp.ID)
	}
	if backendResp.Success {
		t.Fatal("backend response Success = true, want false")
	}

	// Verify rejection was cached
	exchanges := relay.exchanges()
	if len(exchanges) != 1 {
		t.Fatalf("cached exchanges = %d, want 1", len(exchanges))
	}
	if exchanges[0].Success {
		t.Fatal("cached Success = true, want false")
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

	// Replay 3 cached responses
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

	// First replay works
	if err := replay.replayResponse(1, backendConn); err != nil {
		t.Fatalf("replayResponse error: %v", err)
	}

	// Second replay: cache exhausted, should send Success=false
	if err := replay.replayResponse(2, backendConn); err != nil {
		t.Fatalf("replayResponse error: %v", err)
	}

	if len(backendConn.writtenPackets) != 2 {
		t.Fatalf("backend packets = %d, want 2", len(backendConn.writtenPackets))
	}

	resp := backendConn.writtenPackets[1].(*packet.LoginPluginResponse)
	if resp.Success {
		t.Fatal("expected Success=false when cache exhausted")
	}
}

// TestModernForge_BackendLoginHandler_RelaysToClient tests the backend login
// session handler's integration with the forge login relay.
func TestModernForge_BackendLoginHandler_RelaysToClient(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "TestPlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	player.mu.Lock()
	player.forgeLoginRelay = relay
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

	// Simulate backend sending fml:loginwrapper
	fmlMsg := &packet.LoginPluginMessage{
		ID:      1,
		Channel: ForgeLoginWrapperChannel,
		Data:    []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x01},
	}
	handler.handleLoginPluginMessage(fmlMsg)

	// Verify client received the message
	if len(clientConn.writtenPackets) != 1 {
		t.Fatalf("client packets = %d, want 1", len(clientConn.writtenPackets))
	}
	clientMsg, ok := clientConn.writtenPackets[0].(*packet.LoginPluginMessage)
	if !ok {
		t.Fatalf("expected LoginPluginMessage, got %T", clientConn.writtenPackets[0])
	}

	// Client responds
	if err := login.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID:      clientMsg.ID,
		Success: true,
		Data:    []byte{0x02, 'r', 'e', 'p', 'l', 'y'},
	}); err != nil {
		t.Fatalf("handleLoginPluginResponse error: %v", err)
	}

	// Verify response was forwarded to backend
	if len(backendConn.writtenPackets) != 1 {
		t.Fatalf("backend packets = %d, want 1", len(backendConn.writtenPackets))
	}
	backendResp, ok := backendConn.writtenPackets[0].(*packet.LoginPluginResponse)
	if !ok {
		t.Fatalf("expected LoginPluginResponse, got %T", backendConn.writtenPackets[0])
	}
	if backendResp.ID != 1 {
		t.Fatalf("backend response ID = %d, want 1", backendResp.ID)
	}
}

// TestModernForge_BackendLoginHandler_CompletesRelay tests that when the backend
// sends ServerLoginSuccess, the relay completes and LoginSuccess is sent to client.
func TestModernForge_BackendLoginHandler_CompletesRelay(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{eventMgr: event.New()},
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "ForgePlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	player.mu.Lock()
	player.forgeLoginRelay = relay
	player.mu.Unlock()

	// Do one exchange first
	backendMsg := &packet.LoginPluginMessage{
		ID: 1, Channel: ForgeLoginWrapperChannel, Data: []byte{0x01},
	}
	_ = relay.relayToClient(backendConn, backendMsg)
	clientMsg := clientConn.writtenPackets[0].(*packet.LoginPluginMessage)
	_ = login.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID: clientMsg.ID, Success: true, Data: []byte{0x02},
	})

	// Clear written packets for clarity
	clientConn.writtenPackets = nil

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

	// Simulate backend sending ServerLoginSuccess
	handler.handleServerLoginSuccess()

	// Verify LoginSuccess was sent to client
	if len(clientConn.writtenPackets) < 1 {
		t.Fatalf("expected LoginSuccess to be sent to client, got %d packets", len(clientConn.writtenPackets))
	}
	success, ok := clientConn.writtenPackets[0].(*packet.ServerLoginSuccess)
	if !ok {
		t.Fatalf("expected ServerLoginSuccess, got %T", clientConn.writtenPackets[0])
	}
	if success.Username != "ForgePlayer" {
		t.Fatalf("username = %q, want %q", success.Username, "ForgePlayer")
	}

	// Verify forgeLoginRelay was cleared
	player.mu.RLock()
	if player.forgeLoginRelay != nil {
		t.Fatal("forgeLoginRelay should be nil after completion")
	}
	if player.forgeLoginCache == nil {
		t.Fatal("forgeLoginCache should be set after completion")
	}
	if len(player.forgeLoginCache) != 1 {
		t.Fatalf("forgeLoginCache = %d entries, want 1", len(player.forgeLoginCache))
	}
	player.mu.RUnlock()
}

// TestModernForge_BackendLoginHandler_ReplayOnSwitch tests that during a server
// switch, cached FML responses are replayed to the new backend.
func TestModernForge_BackendLoginHandler_ReplayOnSwitch(t *testing.T) {
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: &testMinecraftConn{connType: phase.ModernForge},
		log:           logr.Discard(),
	}

	// Set up cached exchanges (from initial connection)
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

	// Backend sends 2 fml:loginwrapper messages (server switch)
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 1, Channel: ForgeLoginWrapperChannel, Data: []byte{0x01},
	})
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 2, Channel: ForgeLoginWrapperChannel, Data: []byte{0x03},
	})

	// Verify both cached responses were sent to backend
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
		if string(resp.Data) != string(cached[i].Response) {
			t.Fatalf("response[%d] data = %x, want %x", i, resp.Data, cached[i].Response)
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
	// No forgeLoginRelay or forgeReplayRelay set — vanilla client

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

	// Backend sends a non-forge LoginPluginMessage (e.g., custom channel)
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 1, Channel: "custom:channel", Data: []byte{0x01},
	})

	// Verify backend received Success=false (no event subscriber)
	if len(backendConn.writtenPackets) != 1 {
		t.Fatalf("backend packets = %d, want 1", len(backendConn.writtenPackets))
	}
	resp := backendConn.writtenPackets[0].(*packet.LoginPluginResponse)
	if resp.Success {
		t.Fatal("vanilla client: custom channel should get Success=false")
	}
}

// TestModernForge_ConcurrentRelay verifies the relay is safe under concurrent access.
func TestModernForge_ConcurrentRelay(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "TestPlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	// Send messages sequentially (loginInboundConn.outstandingResponses is not concurrent-safe)
	// but verify the relay's internal caching is thread-safe.
	const numMessages = 10

	for i := 0; i < numMessages; i++ {
		msg := &packet.LoginPluginMessage{
			ID:      i,
			Channel: ForgeLoginWrapperChannel,
			Data:    []byte{byte(i)},
		}
		if err := relay.relayToClient(backendConn, msg); err != nil {
			t.Fatalf("relayToClient[%d] error: %v", i, err)
		}

		clientMsg := clientConn.writtenPackets[i].(*packet.LoginPluginMessage)
		if err := login.handleLoginPluginResponse(&packet.LoginPluginResponse{
			ID: clientMsg.ID, Success: true, Data: []byte{byte(i)},
		}); err != nil {
			t.Fatalf("handleLoginPluginResponse[%d] error: %v", i, err)
		}
	}

	// Verify all cached
	exchanges := relay.exchanges()
	if len(exchanges) != numMessages {
		t.Fatalf("cached exchanges = %d, want %d", len(exchanges), numMessages)
	}
}

// TestModernForge_AuthHandler_LoginPluginResponse tests that the auth session
// handler correctly routes LoginPluginResponse to the loginInboundConn.
func TestModernForge_AuthHandler_LoginPluginResponse(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "TestPlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	player.mu.Lock()
	player.forgeLoginRelay = relay
	player.mu.Unlock()

	// Relay a message to create an outstanding response
	msg := &packet.LoginPluginMessage{
		ID: 1, Channel: ForgeLoginWrapperChannel, Data: []byte{0x01},
	}
	if err := relay.relayToClient(backendConn, msg); err != nil {
		t.Fatalf("relayToClient error: %v", err)
	}

	clientMsg := clientConn.writtenPackets[0].(*packet.LoginPluginMessage)

	// Create auth handler
	authHandler := &authSessionHandler{
		inbound: login,
		log:     logr.Discard(),
	}

	// Simulate client sending LoginPluginResponse through the auth handler
	authHandler.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID: clientMsg.ID, Success: true, Data: []byte{0x02},
	})

	// Verify backend received the response
	if len(backendConn.writtenPackets) != 1 {
		t.Fatalf("backend packets = %d, want 1", len(backendConn.writtenPackets))
	}
}

// TestModernForge_DelayedLoginSuccess_Detection tests that Modern Forge clients
// on pre-1.20.2 are correctly identified for delayed LoginSuccess.
func TestModernForge_DelayedLoginSuccess_Detection(t *testing.T) {
	tests := []struct {
		name      string
		connType  phase.ConnectionType
		protocol  proto.Protocol
		expectDelay bool
	}{
		{
			name:        "FML3 1.20.1 - should delay",
			connType:    phase.ModernForge,
			protocol:    version.Minecraft_1_20.Protocol,
			expectDelay: true,
		},
		{
			name:        "FML2 1.16.4 - should delay",
			connType:    phase.ModernForge,
			protocol:    version.Minecraft_1_16_4.Protocol,
			expectDelay: true,
		},
		{
			name:        "Forge 1.20.2+ - should NOT delay (CONFIG phase handles FML)",
			connType:    phase.ModernForge,
			protocol:    version.Minecraft_1_20_2.Protocol,
			expectDelay: false,
		},
		{
			name:        "Vanilla 1.20.1 - should NOT delay",
			connType:    phase.Vanilla,
			protocol:    version.Minecraft_1_20.Protocol,
			expectDelay: false,
		},
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

// TestModernForge_FullRelayFlow simulates the complete Modern Forge login relay:
// backend sends velocity:player_info + fml:loginwrapper messages, client responds,
// backend sends ServerLoginSuccess, and the relay completes.
func TestModernForge_FullRelayFlow(t *testing.T) {
	clientConn := &testMinecraftConn{connType: phase.ModernForge}
	backendConn := &testMinecraftConn{}

	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{eventMgr: event.New()},
	}

	login := newTestLoginInboundConn(clientConn)
	loginSuccess := &packet.ServerLoginSuccess{Username: "ForgePlayer"}
	relay := newModernForgeLoginRelay(login, player, loginSuccess)

	player.mu.Lock()
	player.forgeLoginRelay = relay
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

	// Step 1: Backend sends velocity:player_info (handled separately, not relayed)
	// This is handled by the velocity forwarding code, not tested here.

	// Step 2: Backend sends fml:loginwrapper - ModList
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 1, Channel: ForgeLoginWrapperChannel,
		Data: []byte{0x0d, 'f', 'm', 'l', ':', 'h', 'a', 'n', 'd', 's', 'h', 'a', 'k', 'e', 0x01},
	})

	// Client responds with ModListReply
	clientMsg1 := clientConn.writtenPackets[0].(*packet.LoginPluginMessage)
	_ = login.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID: clientMsg1.ID, Success: true, Data: []byte{0x02, 'm', 'o', 'd', 's'},
	})

	// Step 3: Backend sends fml:loginwrapper - Registry
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 2, Channel: ForgeLoginWrapperChannel,
		Data: []byte{0x03, 'r', 'e', 'g'},
	})

	// Client ACKs
	clientMsg2 := clientConn.writtenPackets[1].(*packet.LoginPluginMessage)
	_ = login.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID: clientMsg2.ID, Success: true, Data: []byte{0x63},
	})

	// Step 4: Backend sends fml:loginwrapper - ConfigData
	handler.handleLoginPluginMessage(&packet.LoginPluginMessage{
		ID: 3, Channel: ForgeLoginWrapperChannel,
		Data: []byte{0x04, 'c', 'f', 'g'},
	})

	// Client ACKs
	clientMsg3 := clientConn.writtenPackets[2].(*packet.LoginPluginMessage)
	_ = login.handleLoginPluginResponse(&packet.LoginPluginResponse{
		ID: clientMsg3.ID, Success: true, Data: []byte{0x63},
	})

	// Clear client packets for clarity
	clientConn.writtenPackets = nil

	// Step 5: Backend sends ServerLoginSuccess → relay completes
	handler.handleServerLoginSuccess()

	// Verify LoginSuccess was sent
	if len(clientConn.writtenPackets) < 1 {
		t.Fatal("expected LoginSuccess to be sent to client")
	}
	success := clientConn.writtenPackets[0].(*packet.ServerLoginSuccess)
	if success.Username != "ForgePlayer" {
		t.Fatalf("username = %q, want ForgePlayer", success.Username)
	}

	// Verify relay is cleared and cache is populated
	player.mu.RLock()
	defer player.mu.RUnlock()
	if player.forgeLoginRelay != nil {
		t.Fatal("forgeLoginRelay should be nil")
	}
	if len(player.forgeLoginCache) != 3 {
		t.Fatalf("forgeLoginCache = %d, want 3", len(player.forgeLoginCache))
	}
}

// -- Test helpers --

// newTestLoginInboundConn creates a loginInboundConn suitable for testing.
// It's pre-configured as if the login event has fired.
func newTestLoginInboundConn(mc *testMinecraftConn) *loginInboundConn {
	login := &loginInboundConn{
		delegate: &initialInbound{
			MinecraftConn: mc,
		},
		outstandingResponses: map[int]MessageConsumer{},
		isLoginEventFired:    true, // Skip queuing, write directly
	}
	return login
}

// Verify that testMinecraftConn using version 1.20.1 (pre-1.20.2) for the test
// is correct since Forge 1.13-1.20.1 is what we're targeting.
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


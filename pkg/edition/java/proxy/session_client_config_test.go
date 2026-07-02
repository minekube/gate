package proxy

import (
	"net"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
)

func newTestConfigHandler(t *testing.T) (*clientConfigSessionHandler, *connectedPlayer, *serverConnection, *testMinecraftConn) {
	t.Helper()

	clientConn := &testMinecraftConn{}
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn: clientConn,
		log:           logr.Discard(),
	}
	proxy := &Proxy{
		event:            event.Nop,
		channelRegistrar: message.NewChannelRegistrar(),
	}
	server := newRegisteredServer(NewServerInfo("purpur", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565}))
	serverConn := newServerConnection(server, nil, player)
	serverConn.connection = backendConn
	player.sessionHandlerDeps = &sessionHandlerDeps{proxy: proxy}

	return newClientConfigSessionHandler(player), player, serverConn, backendConn
}

func TestClientConfigQueuesPluginMessagesUntilBackendReady(t *testing.T) {
	handler, player, serverConn, backendConn := newTestConfigHandler(t)

	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	handler.handlePluginMessage(&plugin.Message{
		Channel: "minecraft:register",
		Data:    []byte("purpur:config"),
	})

	if got := len(backendConn.writtenPackets); got != 0 {
		t.Fatalf("plugin message was written before backend config was ready: got %d packets", got)
	}

	if err := handler.flushQueuedPluginMessagesTo(serverConn); err != nil {
		t.Fatalf("flush queued config plugin messages: %v", err)
	}

	if got := len(backendConn.writtenPackets); got != 1 {
		t.Fatalf("queued plugin messages written to backend = %d, want 1", got)
	}
	got, ok := backendConn.writtenPackets[0].(*plugin.Message)
	if !ok {
		t.Fatalf("queued packet type = %T, want *plugin.Message", backendConn.writtenPackets[0])
	}
	if got.Channel != "minecraft:register" || string(got.Data) != "purpur:config" {
		t.Fatalf("queued packet = %q %q, want minecraft:register purpur:config", got.Channel, string(got.Data))
	}
}

func TestClientConfigForwardsPluginMessagesAfterBackendReady(t *testing.T) {
	handler, player, serverConn, backendConn := newTestConfigHandler(t)

	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	if err := handler.flushQueuedPluginMessagesTo(serverConn); err != nil {
		t.Fatalf("mark backend ready: %v", err)
	}

	handler.handlePluginMessage(&plugin.Message{
		Channel: "minecraft:register",
		Data:    []byte("purpur:ready"),
	})

	if got := len(backendConn.writtenPackets); got != 1 {
		t.Fatalf("direct plugin messages written to backend = %d, want 1", got)
	}
	got := backendConn.writtenPackets[0].(*plugin.Message)
	if got.Channel != "minecraft:register" || string(got.Data) != "purpur:ready" {
		t.Fatalf("direct packet = %q %q, want minecraft:register purpur:ready", got.Channel, string(got.Data))
	}
}

func TestClientConfigBackendFinishReplaysClientBrandAndSwitchesOnlyOutboundState(t *testing.T) {
	handler, player, serverConn, backendConn := newTestConfigHandler(t)
	player.setClientBrand("vanilla")
	handler.handlePluginMessage(&plugin.Message{
		Channel: plugin.BrandChannel,
		Data:    []byte{0x07, 'v', 'a', 'n', 'i', 'l', 'l', 'a'},
	})

	done := handler.handleBackendFinishUpdate(serverConn, &config.FinishedUpdate{})
	if done == nil {
		t.Fatal("handleBackendFinishUpdate returned nil future")
	}

	if got := len(backendConn.writtenPackets); got != 1 {
		t.Fatalf("backend packets = %d, want replayed brand only", got)
	}
	brand, ok := backendConn.writtenPackets[0].(*plugin.Message)
	if !ok {
		t.Fatalf("backend packet type = %T, want *plugin.Message", backendConn.writtenPackets[0])
	}
	if brand.Channel != plugin.BrandChannel || string(plugin.ReadBrandMessage(brand.Data)) != "vanilla" {
		t.Fatalf("brand replay = %q %q, want minecraft:brand vanilla", brand.Channel, plugin.ReadBrandMessage(brand.Data))
	}
	writer := player.Writer().(*testWriter)
	if writer.state != state.Play {
		t.Fatalf("client outbound state = %v, want Play", writer.state)
	}
}

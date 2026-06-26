package proxy

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	javaconfig "go.minekube.com/gate/pkg/edition/java/config"
	cfgpacket "go.minekube.com/gate/pkg/edition/java/proto/packet/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
)

func TestBackendLoginSuccessFlushesQueuedConfigPluginMessages(t *testing.T) {
	clientConn := &testMinecraftConn{}
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	configHandler := newClientConfigSessionHandler(player)
	clientConn.SetActiveSessionHandler(state.Config, configHandler)

	msg := &plugin.Message{
		Channel: "vv:mod_details",
		Data:    []byte(`{"platform":"ViaFabric"}`),
	}
	configHandler.handlePluginMessage(msg)
	if len(backendConn.writtenPackets) != 0 {
		t.Fatalf("backend packets before connection = %d, want 0", len(backendConn.writtenPackets))
	}

	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()
	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	handler := &backendLoginSessionHandler{
		serverConn: serverConn,
		requestCtx: &connRequestCxt{
			Context:  context.Background(),
			response: make(chan *connResponse, 1),
		},
		log: logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{
			eventMgr:       event.New(),
			configProvider: &testConfigProvider{cfg: &javaconfig.Config{}},
		},
	}

	handler.handleServerLoginSuccess()

	for _, pkt := range backendConn.writtenPackets {
		got, ok := pkt.(*plugin.Message)
		if !ok {
			continue
		}
		if got.Channel != msg.Channel {
			t.Fatalf("queued plugin channel = %q, want %q", got.Channel, msg.Channel)
		}
		if string(got.Data) != string(msg.Data) {
			t.Fatalf("queued plugin data = %q, want %q", string(got.Data), string(msg.Data))
		}
		return
	}
	t.Fatalf("queued config plugin message was not flushed to backend; packets: %#v", backendConn.writtenPackets)
}

func TestConfigPluginMessageQueuesWhileBackendConnectionInFlightWithoutSocket(t *testing.T) {
	clientConn := &testMinecraftConn{}
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	configHandler := newClientConfigSessionHandler(player)
	clientConn.SetActiveSessionHandler(state.Config, configHandler)

	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	msg := &plugin.Message{Channel: "vv:mod_details", Data: []byte(`{"phase":"in-flight"}`)}
	configHandler.handlePluginMessage(msg)
	if len(backendConn.writtenPackets) != 0 {
		t.Fatalf("backend packets before socket = %d, want 0", len(backendConn.writtenPackets))
	}

	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()
	handleBackendLoginSuccessForConfigTest(t, serverConn)

	requirePluginMessagesInOrder(t, backendConn.writtenPackets, msg)
}

func TestConfigPluginMessageQueuesUntilBackendConfigReadyAndPreservesOrder(t *testing.T) {
	clientConn := &testMinecraftConn{}
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	configHandler := newClientConfigSessionHandler(player)
	clientConn.SetActiveSessionHandler(state.Config, configHandler)

	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()
	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	first := &plugin.Message{Channel: "vv:mod_details", Data: []byte(`{"order":1}`)}
	second := &plugin.Message{Channel: "fabric:registry", Data: []byte(`{"order":2}`)}
	configHandler.handlePluginMessage(first)
	configHandler.handlePluginMessage(second)
	if len(backendConn.writtenPackets) != 0 {
		t.Fatalf("backend packets before login success = %d, want 0", len(backendConn.writtenPackets))
	}

	handleBackendLoginSuccessForConfigTest(t, serverConn)

	requirePluginMessagesInOrder(t, backendConn.writtenPackets, first, second)
}

func TestConfigPluginMessageForwardsAfterBackendConfigReady(t *testing.T) {
	clientConn := &testMinecraftConn{}
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	configHandler := newClientConfigSessionHandler(player)
	clientConn.SetActiveSessionHandler(state.Config, configHandler)

	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()
	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()

	handleBackendLoginSuccessForConfigTest(t, serverConn)

	msg := &plugin.Message{Channel: "vv:mod_details", Data: []byte(`{"ready":true}`)}
	configHandler.handlePluginMessage(msg)

	requirePluginMessagesInOrder(t, backendConn.writtenPackets, msg)
}

func TestConfigPluginMessageQueuesForNewBackendAttemptAfterPreviousReady(t *testing.T) {
	clientConn := &testMinecraftConn{}
	firstBackendConn := &testMinecraftConn{}
	secondBackendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	configHandler := newClientConfigSessionHandler(player)
	clientConn.SetActiveSessionHandler(state.Config, configHandler)

	firstServerConn := &serverConnection{player: player, log: logr.Discard()}
	firstServerConn.mu.Lock()
	firstServerConn.connection = firstBackendConn
	firstServerConn.mu.Unlock()
	player.mu.Lock()
	player.connInFlight = firstServerConn
	player.mu.Unlock()
	handleBackendLoginSuccessForConfigTest(t, firstServerConn)

	secondServerConn := &serverConnection{player: player, log: logr.Discard()}
	secondServerConn.mu.Lock()
	secondServerConn.connection = secondBackendConn
	secondServerConn.mu.Unlock()
	player.mu.Lock()
	player.connInFlight = secondServerConn
	player.mu.Unlock()

	msg := &plugin.Message{Channel: "vv:mod_details", Data: []byte(`{"retry":true}`)}
	configHandler.handlePluginMessage(msg)
	if len(secondBackendConn.writtenPackets) != 0 {
		t.Fatalf("second backend packets before config ready = %d, want 0", len(secondBackendConn.writtenPackets))
	}

	handleBackendLoginSuccessForConfigTest(t, secondServerConn)

	requirePluginMessagesInOrder(t, secondBackendConn.writtenPackets, msg)
}

func TestClientConfigForwardsLateBrandAfterBackendConfigReady(t *testing.T) {
	clientConn := &testMinecraftConn{}
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      clientConn,
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	configHandler := newClientConfigSessionHandler(player)
	clientConn.SetActiveSessionHandler(state.Config, configHandler)

	serverConn := &serverConnection{player: player, log: logr.Discard()}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()
	player.mu.Lock()
	player.connInFlight = serverConn
	player.mu.Unlock()
	handleBackendLoginSuccessForConfigTest(t, serverConn)

	var data strings.Builder
	if err := util.WriteString(&data, "ViaFabric"); err != nil {
		t.Fatalf("write brand: %v", err)
	}
	configHandler.handlePluginMessage(&plugin.Message{
		Channel: plugin.BrandChannel,
		Data:    []byte(data.String()),
	})

	requirePluginMessagesInOrder(t, backendConn.writtenPackets, &plugin.Message{
		Channel: plugin.BrandChannel,
		Data:    []byte(data.String()),
	})
}

func TestClientConfigStoresBrandPluginMessage(t *testing.T) {
	player := &connectedPlayer{
		MinecraftConn:      &testMinecraftConn{},
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	handler := newClientConfigSessionHandler(player)

	var data strings.Builder
	if err := util.WriteString(&data, "ViaFabric"); err != nil {
		t.Fatalf("write brand: %v", err)
	}
	handler.handlePluginMessage(&plugin.Message{
		Channel: plugin.BrandChannel,
		Data:    []byte(data.String()),
	})

	if got := player.ClientBrand(); got != "ViaFabric" {
		t.Fatalf("client brand = %q, want ViaFabric", got)
	}
}

func TestClientConfigDropsKnownPacksWhenNoTargetServer(t *testing.T) {
	player := &connectedPlayer{
		MinecraftConn:      &testMinecraftConn{},
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	handler := newClientConfigSessionHandler(player)

	handler.handleKnownPacks(&cfgpacket.KnownPacks{}, nil)
}

func handleBackendLoginSuccessForConfigTest(t *testing.T, serverConn *serverConnection) {
	t.Helper()
	handler := &backendLoginSessionHandler{
		serverConn: serverConn,
		requestCtx: &connRequestCxt{
			Context:  context.Background(),
			response: make(chan *connResponse, 1),
		},
		log: logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{
			eventMgr:       event.New(),
			configProvider: &testConfigProvider{cfg: &javaconfig.Config{}},
		},
	}
	handler.handleServerLoginSuccess()
}

func requirePluginMessagesInOrder(t *testing.T, packets []proto.Packet, want ...*plugin.Message) {
	t.Helper()
	got := make([]*plugin.Message, 0, len(want))
	for _, pkt := range packets {
		msg, ok := pkt.(*plugin.Message)
		if ok {
			got = append(got, msg)
		}
	}
	if len(got) < len(want) {
		t.Fatalf("plugin messages = %d, want at least %d; packets: %#v", len(got), len(want), packets)
	}
	got = got[len(got)-len(want):]
	for i, msg := range want {
		if got[i].Channel != msg.Channel {
			t.Fatalf("plugin[%d] channel = %q, want %q", i, got[i].Channel, msg.Channel)
		}
		if string(got[i].Data) != string(msg.Data) {
			t.Fatalf("plugin[%d] data = %q, want %q", i, string(got[i].Data), string(msg.Data))
		}
	}
}

func newConfigTestProxy() *Proxy {
	return &Proxy{
		event:            event.Nop,
		channelRegistrar: message.NewChannelRegistrar(),
	}
}

func TestClientConfigForwardsStoredBrandOnBackendFinishUpdate(t *testing.T) {
	backendConn := &testMinecraftConn{}
	player := &connectedPlayer{
		MinecraftConn:      &testMinecraftConn{},
		log:                logr.Discard(),
		sessionHandlerDeps: &sessionHandlerDeps{proxy: newConfigTestProxy()},
	}
	handler := newClientConfigSessionHandler(player)

	var data strings.Builder
	if err := util.WriteString(&data, "ViaFabric"); err != nil {
		t.Fatalf("write brand: %v", err)
	}
	handler.handlePluginMessage(&plugin.Message{
		Channel: plugin.BrandChannel,
		Data:    []byte(data.String()),
	})

	serverConn := &serverConnection{
		player: player,
		log:    logr.Discard(),
	}
	serverConn.mu.Lock()
	serverConn.connection = backendConn
	serverConn.mu.Unlock()

	if fut := handler.handleBackendFinishUpdate(serverConn, &cfgpacket.FinishedUpdate{}); fut == nil {
		t.Fatal("handleBackendFinishUpdate returned nil future")
	}

	for _, pkt := range backendConn.writtenPackets {
		got, ok := pkt.(*plugin.Message)
		if !ok {
			continue
		}
		if got.Channel != plugin.BrandChannel {
			t.Fatalf("brand channel = %q, want %q", got.Channel, plugin.BrandChannel)
		}
		if brand := plugin.ReadBrandMessage(got.Data); brand != "ViaFabric" {
			t.Fatalf("forwarded brand = %q, want ViaFabric", brand)
		}
		return
	}
	t.Fatalf("stored brand was not forwarded to backend; packets: %#v", backendConn.writtenPackets)
}

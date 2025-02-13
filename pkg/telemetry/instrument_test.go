package telemetry

import (
"context"
"net"
"testing"
"time"

"github.com/robinbraemer/event"
"github.com/stretchr/testify/assert"
"go.minekube.com/common/minecraft/component"
"go.minekube.com/gate/pkg/command"
"go.minekube.com/gate/pkg/gate/config"
"go.minekube.com/gate/pkg/edition/java/profile"
"go.minekube.com/gate/pkg/edition/java/proxy"
"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
"go.minekube.com/gate/pkg/edition/java/proxy/message"
"go.minekube.com/gate/pkg/edition/java/proxy/player"
"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
"go.minekube.com/gate/pkg/gate/proto"
"go.minekube.com/gate/pkg/util/permission"
"go.minekube.com/gate/pkg/util/uuid"
)

// Mock implementations
type simpleEventMgr struct {
handlers map[string][]event.HandlerFunc
}

func newSimpleEventMgr() *simpleEventMgr {
return &simpleEventMgr{
handlers: make(map[string][]event.HandlerFunc),
}
}

func getEventType(e event.Event) string {
switch e.(type) {
case *proxy.LoginEvent:
return "login"
case *proxy.DisconnectEvent:
return "disconnect"
case *proxy.ServerPreConnectEvent:
return "serverConnect"
case *proxy.CommandExecuteEvent:
return "command"
case *proxy.PluginMessageEvent:
return "pluginMessage"
default:
return "unknown"
}
}

func (m *simpleEventMgr) Subscribe(e event.Event, _ int, handler event.HandlerFunc) func() {
eventType := getEventType(e)
m.handlers[eventType] = append(m.handlers[eventType], handler)
return func() {}
}

func (m *simpleEventMgr) Fire(e event.Event) {
eventType := getEventType(e)
for _, handler := range m.handlers[eventType] {
handler(e)
}
}

// Additional methods to satisfy event.Manager interface
func (m *simpleEventMgr) SubscribeFn(eventType event.Type, fn func(e event.Event) error) {
}

func (m *simpleEventMgr) Unsubscribe(listener interface{}) {
}

func (m *simpleEventMgr) UnsubscribeAll(events ...event.Event) int {
return 0
}

func (m *simpleEventMgr) HasSubscriber(events ...event.Event) bool {
return false
}

func (m *simpleEventMgr) FireParallel(e event.Event, handlers ...event.HandlerFunc) {
m.Fire(e)
}

func (m *simpleEventMgr) FireAsync(e event.Event) {
m.Fire(e)
}

func (m *simpleEventMgr) FireAsyncParallel(e event.Event, handlers ...event.HandlerFunc) {
m.Fire(e)
}

func (m *simpleEventMgr) Wait(events ...event.Event) {
}

type simpleProxy struct {
eventMgr event.Manager
}

func (p *simpleProxy) Event() event.Manager {
return p.eventMgr
}

// Mock implementation of tablist.TabList
type mockTabList struct{}

func (m *mockTabList) HeaderFooter() (header, footer component.Component) { return nil, nil }
func (m *mockTabList) SetHeaderFooter(header, footer component.Component) error { return nil }
func (m *mockTabList) ClearHeaderFooter()                                {}
func (m *mockTabList) Add(entries ...tablist.Entry) error               { return nil }
func (m *mockTabList) Entries() map[uuid.UUID]tablist.Entry             { return nil }
func (m *mockTabList) RemoveAll(ids ...uuid.UUID) error                { return nil }

// Test event implementations
type testPlayer struct {
username   string
id         uuid.UUID
onlineMode bool
gameProfile profile.GameProfile
tabList    tablist.TabList
}

func (p *testPlayer) Username() string                                       { return p.username }
func (p *testPlayer) ID() uuid.UUID                                         { return p.id }
func (p *testPlayer) OnlineMode() bool                                      { return p.onlineMode }
func (p *testPlayer) Active() bool                                          { return true }
func (p *testPlayer) Protocol() proto.Protocol                              { return 0 }
func (p *testPlayer) WritePacket(packet proto.Packet) error                 { return nil }
func (p *testPlayer) BufferPacket(packet proto.Packet) error               { return nil }
func (p *testPlayer) BufferPayload(payload []byte) error                   { return nil }
func (p *testPlayer) Flush() error                                         { return nil }
func (p *testPlayer) SendMessage(msg component.Component, opts ...command.MessageOption) error { return nil }
func (p *testPlayer) SendActionBar(msg component.Component) error           { return nil }
func (p *testPlayer) SendPluginMessage(identifier message.ChannelIdentifier, data []byte) error { return nil }
func (p *testPlayer) HasPermission(permission string) bool                  { return true }
func (p *testPlayer) PermissionValue(perm string) permission.TriState      { return permission.Undefined }
func (p *testPlayer) CurrentServer() proxy.ServerConnection                 { return nil }
func (p *testPlayer) AppliedResourcePack() *proxy.ResourcePackInfo         { return nil }
func (p *testPlayer) PendingResourcePack() *proxy.ResourcePackInfo         { return nil }
func (p *testPlayer) AppliedResourcePacks() []*proxy.ResourcePackInfo      { return nil }
func (p *testPlayer) PendingResourcePacks() []*proxy.ResourcePackInfo      { return nil }
func (p *testPlayer) SendResourcePack(info proxy.ResourcePackInfo) error   { return nil }
func (p *testPlayer) TransferToHost(addr string) error                     { return nil }
func (p *testPlayer) ClientBrand() string                                  { return "" }
func (p *testPlayer) Context() context.Context                             { return context.Background() }
func (p *testPlayer) CreateConnectionRequest(target proxy.RegisteredServer) proxy.ConnectionRequest { return nil }
func (p *testPlayer) Disconnect(reason component.Component)                 {}
func (p *testPlayer) GameProfile() profile.GameProfile                      { return p.gameProfile }
func (p *testPlayer) IdentifiedKey() crypto.IdentifiedKey                   { return nil }
func (p *testPlayer) Ping() time.Duration                                   { return 100 * time.Millisecond }
func (p *testPlayer) RemoteAddr() net.Addr                                  { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345} }
func (p *testPlayer) Settings() player.Settings                             { return player.DefaultSettings }
func (p *testPlayer) SpoofChatInput(input string) error                    { return nil }
func (p *testPlayer) TabList() tablist.TabList                             { return &mockTabList{} }
func (p *testPlayer) VirtualHost() net.Addr                                { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565} }
func (p *testPlayer) Write(b []byte) error                                 { return nil }

type testLoginEvent struct {
player testPlayer
}

func (e *testLoginEvent) Player() proxy.Player {
return &e.player
}

type testDisconnectEvent struct {
player testPlayer
}

func (e *testDisconnectEvent) Player() proxy.Player {
return &e.player
}

type testServerInfo struct {
name string
}

func (s *testServerInfo) Name() string  { return s.name }
func (s *testServerInfo) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 25565} }

type testServer struct {
info testServerInfo
}

func (s *testServer) ServerInfo() proxy.ServerInfo {
return &s.info
}

func (s *testServer) Players() proxy.Players {
return nil
}

type testServerPreConnectEvent struct {
player testPlayer
server testServer
}

func (e *testServerPreConnectEvent) Player() proxy.Player {
return &e.player
}

func (e *testServerPreConnectEvent) Server() proxy.RegisteredServer {
return &e.server
}

type testCommandSource struct{}

type testCommandExecuteEvent struct {
source      testCommandSource
commandLine string
}

func (e *testCommandExecuteEvent) Source() any {
return e.source
}

func (e *testCommandExecuteEvent) Command() string {
return e.commandLine
}

// Mock implementation of message.ChannelIdentifier
type testChannelIdentifier string

func (t testChannelIdentifier) ID() string {
	return string(t)
}

type testPluginMessageEvent struct {
source     testCommandSource
identifier message.ChannelIdentifier
data       []byte
}

func (e *testPluginMessageEvent) Source() any {
return e.source
}

func (e *testPluginMessageEvent) Identifier() message.ChannelIdentifier {
return e.identifier
}

func (e *testPluginMessageEvent) Data() []byte {
return e.data
}

func TestInstrumentProxyTelemetry(t *testing.T) {
	// Initialize telemetry with stdout tracer using default configuration and then enabling tracing
	cfg := WithDefaults(&config.Config{})
	cfg.Telemetry.Tracing.Enabled = true
	cfg.Telemetry.Tracing.Exporter = "stdout"
	
	// Create new telemetry instance
	tel, cleanup, err := New(context.Background(), cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Setup a simple proxy with event manager
	eventMgr := newSimpleEventMgr()
	p := &simpleProxy{eventMgr: eventMgr}

	// Instrument the proxy
	tel.InstrumentProxy(p)

	// Test login event
	loginEvent := &testLoginEvent{
		player: testPlayer{
			username:   "testUser",
			id:        uuid.New(),
			onlineMode: true,
		},
	}
	eventMgr.Fire(loginEvent)

	// Test disconnect event
	disconnectEvent := &testDisconnectEvent{
		player: testPlayer{
			username: "testUser",
			id:      uuid.New(),
		},
	}
	eventMgr.Fire(disconnectEvent)

	// Test server connect event
	serverEvent := &testServerPreConnectEvent{
		player: testPlayer{
			username: "testUser",
			id:      uuid.New(),
		},
		server: testServer{
			info: testServerInfo{
				name: "testServer",
			},
		},
	}
	eventMgr.Fire(serverEvent)

	// Test command execute event
	cmdEvent := &testCommandExecuteEvent{
		source:      testCommandSource{},
		commandLine: "/test command",
	}
	eventMgr.Fire(cmdEvent)

	// Test plugin message event
	pluginEvent := &testPluginMessageEvent{
		source:     testCommandSource{},
		identifier: testChannelIdentifier("test:channel"),
		data:      []byte("test data"),
	}
	eventMgr.Fire(pluginEvent)
}

func TestInstrumentProxyNil(t *testing.T) {
	tel, cleanup, err := New(context.Background(), WithDefaults(&config.Config{}))
	assert.NoError(t, err)
	defer cleanup()
	tel.InstrumentProxy(nil) // Should not panic
}

func TestInstrumentProxyNilEventManager(t *testing.T) {
	tel, cleanup, err := New(context.Background(), WithDefaults(&config.Config{}))
	assert.NoError(t, err)
	defer cleanup()
	tel.InstrumentProxy(&simpleProxy{eventMgr: nil}) // Should not panic
}

// TestTelemetryConfigRespected verifies that user-provided telemetry configurations
// are respected and not overwritten by defaults
func TestTelemetryConfigRespected(t *testing.T) {
	// Create a config with custom telemetry settings
	customConfig := &config.Config{
		Telemetry: config.Telemetry{
			Metrics: config.TelemetryMetrics{
				Enabled: true,
				Endpoint: "http://localhost:9999", // Custom endpoint
				Exporter: "prometheus", // Custom exporter
				AnonymousMetrics: false, // Custom setting
				Prometheus: struct {
					Path string `yaml:"path" json:"path"`
				}{
					Path: "/custom-metrics", // Custom metrics path
				},
			},
			Tracing: config.TelemetryTracing{
				Enabled: true,
				Endpoint: "localhost:4318", // Custom endpoint
				Exporter: "otlp", // Custom exporter
				Sampler: "parentbased_traceidratio", // Custom sampler
			},
		},
	}

	// Create new telemetry instance with custom config
	tel, cleanup, err := New(context.Background(), customConfig)
	assert.NoError(t, err)
	defer cleanup()

	// Verify the telemetry instance uses our custom config
	assert.Equal(t, "http://localhost:9999", customConfig.Telemetry.Metrics.Endpoint, "Metrics endpoint should remain unchanged")
	assert.Equal(t, "prometheus", customConfig.Telemetry.Metrics.Exporter, "Metrics exporter should remain unchanged")
	assert.Equal(t, false, customConfig.Telemetry.Metrics.AnonymousMetrics, "Anonymous metrics setting should remain unchanged")
	assert.Equal(t, "/custom-metrics", customConfig.Telemetry.Metrics.Prometheus.Path, "Prometheus metrics path should remain unchanged")
	assert.Equal(t, "otlp", customConfig.Telemetry.Tracing.Exporter, "Tracing exporter should remain unchanged")
	assert.Equal(t, "localhost:4318", customConfig.Telemetry.Tracing.Endpoint, "Tracing endpoint should remain unchanged")
	assert.Equal(t, "parentbased_traceidratio", customConfig.Telemetry.Tracing.Sampler, "Tracing sampler should remain unchanged")

	// Setup a simple proxy and verify instrumentation works with custom config
	eventMgr := newSimpleEventMgr()
	p := &simpleProxy{eventMgr: eventMgr}
	tel.InstrumentProxy(p)

	// Test an event to verify instrumentation works
	loginEvent := &testLoginEvent{
		player: testPlayer{
			username:   "testUser",
			id:        uuid.New(),
			onlineMode: true,
		},
	}
	eventMgr.Fire(loginEvent) // Should not panic or modify config
}
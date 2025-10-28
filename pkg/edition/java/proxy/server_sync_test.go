package proxy

import (
	"net"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/netutil"
)

// TestServerSyncPreservesAPIServers tests that API-registered servers are preserved
// during config reloads while config servers are properly synced.
func TestServerSyncPreservesAPIServers(t *testing.T) {
	// Create proxy with initial config servers
	proxy := createTestProxy(t, map[string]string{
		"config-server1": "localhost:25565",
		"config-server2": "localhost:25566",
	})

	// Register servers via API (simulating external registration)
	apiServer1 := NewServerInfo("api-server1", mustParseAddr("localhost:25567"))
	apiServer2 := NewServerInfo("api-server2", mustParseAddr("localhost:25568"))

	_, err := proxy.Register(apiServer1)
	if err != nil {
		t.Fatalf("Failed to register API server1: %v", err)
	}
	_, err = proxy.Register(apiServer2)
	if err != nil {
		t.Fatalf("Failed to register API server2: %v", err)
	}

	// Verify all servers are registered
	if len(proxy.Servers()) != 4 {
		t.Fatalf("Expected 4 servers, got %d", len(proxy.Servers()))
	}

	// Update config: remove config-server2, add config-server3
	proxy.cfg.Servers = map[string]string{
		"config-server1": "localhost:25565", // kept
		"config-server3": "localhost:25569", // new
		// config-server2 removed
	}

	// Trigger config reload
	if err := proxy.init(); err != nil {
		t.Fatalf("Failed to sync servers: %v", err)
	}

	// Verify results
	servers := proxy.Servers()
	serverNames := make([]string, len(servers))
	for i, s := range servers {
		serverNames[i] = s.ServerInfo().Name()
	}

	// Should have: config-server1, config-server3, api-server1, api-server2
	expectedServers := []string{"config-server1", "config-server3", "api-server1", "api-server2"}
	if len(servers) != len(expectedServers) {
		t.Errorf("Expected %d servers, got %d. Servers: %v", len(expectedServers), len(servers), serverNames)
	}

	// Verify specific servers
	for _, expected := range expectedServers {
		if proxy.Server(expected) == nil {
			t.Errorf("Expected server %q to be registered", expected)
		}
	}

	// Verify removed server is gone
	if proxy.Server("config-server2") != nil {
		t.Error("config-server2 should have been unregistered")
	}
}

// TestServerSyncOnConfigReload tests that servers are properly registered and unregistered
// when the configuration changes during reload.
func TestServerSyncOnConfigReload(t *testing.T) {
	tests := []struct {
		name            string
		initialServers  map[string]string
		updatedServers  map[string]string
		expectedAdded   []string
		expectedRemoved []string
		expectedKept    []string
		description     string
	}{
		{
			name:           "add_new_server",
			initialServers: map[string]string{"server1": "localhost:25565"},
			updatedServers: map[string]string{"server1": "localhost:25565", "server2": "localhost:25566"},
			expectedAdded:  []string{"server2"},
			expectedKept:   []string{"server1"},
			description:    "Should register new server while keeping existing ones",
		},
		{
			name:            "remove_server",
			initialServers:  map[string]string{"server1": "localhost:25565", "server2": "localhost:25566"},
			updatedServers:  map[string]string{"server1": "localhost:25565"},
			expectedRemoved: []string{"server2"},
			expectedKept:    []string{"server1"},
			description:     "Should unregister removed server while keeping existing ones",
		},
		{
			name:            "remove_all_servers",
			initialServers:  map[string]string{"server1": "localhost:25565", "server2": "localhost:25566"},
			updatedServers:  map[string]string{},
			expectedRemoved: []string{"server1", "server2"},
			description:     "Should unregister all servers when config is empty",
		},
		{
			name:           "change_server_address",
			initialServers: map[string]string{"server1": "localhost:25565"},
			updatedServers: map[string]string{"server1": "localhost:25567"},
			expectedKept:   []string{"server1"}, // Same name, different address
			description:    "Should update server address when changed in config",
		},
		{
			name:            "replace_servers",
			initialServers:  map[string]string{"server1": "localhost:25565", "server2": "localhost:25566"},
			updatedServers:  map[string]string{"server3": "localhost:25567", "server4": "localhost:25568"},
			expectedAdded:   []string{"server3", "server4"},
			expectedRemoved: []string{"server1", "server2"},
			description:     "Should replace all servers with new ones",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create proxy with initial configuration
			proxy := createTestProxy(t, tt.initialServers)

			// Verify initial servers are registered
			for serverName := range tt.initialServers {
				if proxy.Server(serverName) == nil {
					t.Errorf("Initial server %q should be registered", serverName)
				}
			}

			// Update configuration
			proxy.cfg.Servers = tt.updatedServers

			// Trigger server sync (same as config reload)
			if err := proxy.init(); err != nil {
				t.Fatalf("Failed to sync servers: %v", err)
			}

			// Verify expected servers are added/kept
			expectedPresent := append(tt.expectedAdded, tt.expectedKept...)
			for _, serverName := range expectedPresent {
				if proxy.Server(serverName) == nil {
					t.Errorf("Server %q should be registered after sync", serverName)
				}
			}

			// Verify expected servers are removed
			for _, serverName := range tt.expectedRemoved {
				if proxy.Server(serverName) != nil {
					t.Errorf("Server %q should be unregistered after sync", serverName)
				}
			}

			// Verify total server count matches expected
			allServers := proxy.Servers()
			expectedCount := len(tt.updatedServers)
			if len(allServers) != expectedCount {
				t.Errorf("Expected %d servers, got %d", expectedCount, len(allServers))
			}
		})
	}
}

// TestServerSyncCaseInsensitive tests that server name comparison is case-insensitive
// during sync operations.
func TestServerSyncCaseInsensitive(t *testing.T) {
	// Create proxy with mixed-case server names
	initialServers := map[string]string{
		"Server1": "localhost:25565",
		"SERVER2": "localhost:25566",
	}
	proxy := createTestProxy(t, initialServers)

	// Update config with different case
	proxy.cfg.Servers = map[string]string{
		"server1": "localhost:25565", // lowercase
		"server3": "localhost:25567", // new server
	}

	// Trigger sync
	if err := proxy.init(); err != nil {
		t.Fatalf("Failed to sync servers: %v", err)
	}

	// Verify case-insensitive matching worked
	if proxy.Server("server1") == nil {
		t.Error("server1 should be registered (case-insensitive match)")
	}
	if proxy.Server("Server1") == nil {
		t.Error("Server1 should be accessible (case-insensitive lookup)")
	}
	if proxy.Server("SERVER2") != nil {
		t.Error("SERVER2 should be unregistered")
	}
	if proxy.Server("server3") == nil {
		t.Error("server3 should be registered")
	}

	// Verify total count
	allServers := proxy.Servers()
	if len(allServers) != 2 {
		t.Errorf("Expected 2 servers, got %d", len(allServers))
	}
}

// TestServerSyncWithAddressChange tests that changing a server's address
// properly updates the registration.
func TestServerSyncWithAddressChange(t *testing.T) {
	proxy := createTestProxy(t, map[string]string{
		"testserver": "localhost:25565",
	})

	// Get initial server info
	initialServer := proxy.Server("testserver")
	if initialServer == nil {
		t.Fatal("Initial server should be registered")
	}
	initialAddr := initialServer.ServerInfo().Addr().String()

	// Change server address in config
	proxy.cfg.Servers = map[string]string{
		"testserver": "localhost:25567", // Different port
	}

	// Trigger sync
	if err := proxy.init(); err != nil {
		t.Fatalf("Failed to sync servers: %v", err)
	}

	// Verify server still exists but with new address
	updatedServer := proxy.Server("testserver")
	if updatedServer == nil {
		t.Fatal("Server should still be registered after address change")
	}

	updatedAddr := updatedServer.ServerInfo().Addr().String()
	if updatedAddr == initialAddr {
		t.Error("Server address should have been updated")
	}
	if !strings.Contains(updatedAddr, "25567") {
		t.Errorf("Expected new address to contain port 25567, got %s", updatedAddr)
	}
}

// createTestProxy creates a test proxy with the given server configuration
func createTestProxy(t *testing.T, servers map[string]string) *Proxy {
	cfg := &config.Config{
		Servers: servers,
		Lite:    liteconfig.Config{Enabled: false}, // Disable lite mode for server registration
	}

	// Create a minimal authenticator for testing
	authenticator, err := auth.New(auth.Options{})
	if err != nil {
		t.Fatalf("Failed to create authenticator: %v", err)
	}

	proxy := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		authenticator: authenticator,
	}

	// Initialize with initial servers
	if err := proxy.init(); err != nil {
		t.Fatalf("Failed to initialize proxy: %v", err)
	}

	return proxy
}

// mustParseAddr is a helper function for tests
func mustParseAddr(addr string) net.Addr {
	parsed, err := netutil.Parse(addr, "tcp")
	if err != nil {
		panic(err)
	}
	return parsed
}

// TestServerInfoEqual tests the ServerInfoEqual function used in server sync
func TestServerInfoEqual(t *testing.T) {
	addr1, _ := netutil.Parse("localhost:25565", "tcp")
	addr2, _ := netutil.Parse("localhost:25566", "tcp")

	info1 := NewServerInfo("server1", addr1)
	info2 := NewServerInfo("server1", addr1) // Same name and address
	info3 := NewServerInfo("server1", addr2) // Same name, different address
	info4 := NewServerInfo("server2", addr1) // Different name, same address

	tests := []struct {
		name     string
		info1    ServerInfo
		info2    ServerInfo
		expected bool
	}{
		{"identical_servers", info1, info2, true},
		{"same_name_different_address", info1, info3, false},
		{"different_name_same_address", info1, info4, false},
		{"same_server_instance", info1, info1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ServerInfoEqual(tt.info1, tt.info2)
			if result != tt.expected {
				t.Errorf("ServerInfoEqual(%v, %v) = %v, expected %v",
					tt.info1, tt.info2, result, tt.expected)
			}
		})
	}
}

// TestServerRegistrationEvents tests that ServerRegisteredEvent and ServerUnregisteredEvent
// are fired when servers are registered and unregistered.
func TestServerRegistrationEvents(t *testing.T) {
	eventMgr := event.New()
	
	// Track events
	var registeredServers []string
	var unregisteredServers []string
	
	// Subscribe to ServerRegisteredEvent
	event.Subscribe(eventMgr, 0, func(e *ServerRegisteredEvent) {
		registeredServers = append(registeredServers, e.Server().ServerInfo().Name())
	})
	
	// Subscribe to ServerUnregisteredEvent
	event.Subscribe(eventMgr, 0, func(e *ServerUnregisteredEvent) {
		unregisteredServers = append(unregisteredServers, e.ServerInfo().Name())
	})
	
	// Create proxy with event manager
	cfg := &config.Config{
		Servers: map[string]string{},
		Lite:    liteconfig.Config{Enabled: false},
	}
	
	authenticator, err := auth.New(auth.Options{})
	if err != nil {
		t.Fatalf("Failed to create authenticator: %v", err)
	}
	
	proxy := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         eventMgr,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		authenticator: authenticator,
	}
	
	// Register a server
	serverInfo := NewServerInfo("test-server", mustParseAddr("localhost:25565"))
	_, err = proxy.Register(serverInfo)
	if err != nil {
		t.Fatalf("Failed to register server: %v", err)
	}
	
	// Wait for event to process
	eventMgr.Wait()
	
	// Verify ServerRegisteredEvent was fired
	if len(registeredServers) != 1 {
		t.Errorf("Expected 1 registered event, got %d", len(registeredServers))
	}
	if len(registeredServers) > 0 && registeredServers[0] != "test-server" {
		t.Errorf("Expected registered server to be 'test-server', got '%s'", registeredServers[0])
	}
	
	// Unregister the server
	if !proxy.Unregister(serverInfo) {
		t.Fatal("Failed to unregister server")
	}
	
	// Wait for event to process
	eventMgr.Wait()
	
	// Verify ServerUnregisteredEvent was fired
	if len(unregisteredServers) != 1 {
		t.Errorf("Expected 1 unregistered event, got %d", len(unregisteredServers))
	}
	if len(unregisteredServers) > 0 && unregisteredServers[0] != "test-server" {
		t.Errorf("Expected unregistered server to be 'test-server', got '%s'", unregisteredServers[0])
	}
}

// TestServerRegistrationEventsMultiple tests that events are fired for multiple server operations
func TestServerRegistrationEventsMultiple(t *testing.T) {
	eventMgr := event.New()
	
	// Track events
	registeredCount := 0
	unregisteredCount := 0
	
	// Subscribe to events
	event.Subscribe(eventMgr, 0, func(e *ServerRegisteredEvent) {
		registeredCount++
	})
	
	event.Subscribe(eventMgr, 0, func(e *ServerUnregisteredEvent) {
		unregisteredCount++
	})
	
	// Create proxy with event manager
	cfg := &config.Config{
		Servers: map[string]string{},
		Lite:    liteconfig.Config{Enabled: false},
	}
	
	authenticator, err := auth.New(auth.Options{})
	if err != nil {
		t.Fatalf("Failed to create authenticator: %v", err)
	}
	
	proxy := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         eventMgr,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		authenticator: authenticator,
	}
	
	// Register multiple servers
	servers := []ServerInfo{
		NewServerInfo("server1", mustParseAddr("localhost:25565")),
		NewServerInfo("server2", mustParseAddr("localhost:25566")),
		NewServerInfo("server3", mustParseAddr("localhost:25567")),
	}
	
	for _, s := range servers {
		_, err := proxy.Register(s)
		if err != nil {
			t.Fatalf("Failed to register server %s: %v", s.Name(), err)
		}
	}
	
	eventMgr.Wait()
	
	// Verify all registration events were fired
	if registeredCount != 3 {
		t.Errorf("Expected 3 registered events, got %d", registeredCount)
	}
	
	// Unregister all servers
	for _, s := range servers {
		if !proxy.Unregister(s) {
			t.Errorf("Failed to unregister server %s", s.Name())
		}
	}
	
	eventMgr.Wait()
	
	// Verify all unregistration events were fired
	if unregisteredCount != 3 {
		t.Errorf("Expected 3 unregistered events, got %d", unregisteredCount)
	}
}

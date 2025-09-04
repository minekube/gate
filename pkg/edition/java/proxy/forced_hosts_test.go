package proxy

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/netutil"
)

func TestForcedHosts_NextServerToTry(t *testing.T) {
	// Create a test proxy with servers and forced hosts configuration
	proxy := createTestProxyWithForcedHosts(t, map[string]string{
		"server1": "localhost:25566",
		"server2": "localhost:25567",
		"server3": "localhost:25568",
	}, map[string][]string{
		"play.example.com": {"server1", "server2"}, // Hostname only (matching Velocity behavior)
	}, []string{"server3"}) // Default Try list

	// Create a mock player with the virtual host that has forced hosts configured
	player := &connectedPlayer{
		sessionHandlerDeps: &sessionHandlerDeps{
			proxy:          proxy,
			configProvider: &testConfigProvider{cfg: proxy.cfg},
		},
		virtualHost:  netutil.NewAddr("play.example.com:25565", "tcp"), // Full address
		serversToTry: nil, // Initially empty
		tryIndex:     0,
	}

	// Test: First call should return server1 (first forced host)
	firstServer := player.nextServerToTry(nil)
	require.NotNil(t, firstServer, "Should return the first forced host server")
	assert.Equal(t, "server1", firstServer.ServerInfo().Name(), "Should return server1 as the first forced host")

	// Verify serversToTry was populated with forced hosts
	assert.Equal(t, []string{"server1", "server2"}, player.serversToTry, "serversToTry should be populated with forced hosts")

	// Test: Second call should return server2 (second forced host)
	player.tryIndex++ // Simulate server1 connection failure
	secondServer := player.nextServerToTry(firstServer)
	require.NotNil(t, secondServer, "Should return the second forced host server")
	assert.Equal(t, "server2", secondServer.ServerInfo().Name(), "Should return server2 as the second forced host")

	// Test: Third call should return nil (no more forced hosts available)
	player.tryIndex++ // Simulate server2 connection failure
	thirdServer := player.nextServerToTry(secondServer)
	assert.Nil(t, thirdServer, "Should return nil when no more servers available")
}

func TestForcedHosts_FallbackToTryList(t *testing.T) {
	// Create a proxy WITHOUT forced hosts for our virtual host
	proxy := createTestProxyWithForcedHosts(t, map[string]string{
		"server1": "localhost:25566",
		"server3": "localhost:25568",
	}, map[string][]string{
		"other.example.com": {"server1"}, // Different hostname, not matching
	}, []string{"server3"}) // Default Try list

	// Create a mock player with a virtual host that has NO forced hosts configured
	player := &connectedPlayer{
		sessionHandlerDeps: &sessionHandlerDeps{
			proxy:          proxy,
			configProvider: &testConfigProvider{cfg: proxy.cfg},
		},
		virtualHost:  netutil.NewAddr("play.example.com:25565", "tcp"), // No forced hosts for this hostname
		serversToTry: nil, // Initially empty
		tryIndex:     0,
	}

	// Test: Should fall back to Try list since no forced hosts match
	firstServer := player.nextServerToTry(nil)
	require.NotNil(t, firstServer, "Should return server from Try list as fallback")
	assert.Equal(t, "server3", firstServer.ServerInfo().Name(), "Should return server3 from Try list")

	// Verify serversToTry was populated with Try list
	assert.Equal(t, []string{"server3"}, player.serversToTry, "serversToTry should be populated with Try list as fallback")
}

func TestForcedHosts_VirtualHostProcessing(t *testing.T) {
	// Test different virtual host formats to ensure proper hostname extraction
	testCases := []struct {
		name              string
		virtualHost       string
		configKey         string
		shouldUseForcedHost bool
		description       string
	}{
		{
			name:              "exact hostname match",
			virtualHost:       "play.example.com:25565",
			configKey:         "play.example.com",
			shouldUseForcedHost: true,
			description:       "Virtual host with port should match hostname-only config key",
		},
		{
			name:              "case insensitive matching",
			virtualHost:       "PLAY.EXAMPLE.COM:25565",
			configKey:         "play.example.com",
			shouldUseForcedHost: true,
			description:       "Case should be ignored in hostname matching",
		},
		{
			name:              "different hostname",
			virtualHost:       "different.example.com:25565",
			configKey:         "play.example.com",
			shouldUseForcedHost: false,
			description:       "Different hostname should not match",
		},
		{
			name:              "hostname without port",
			virtualHost:       "play.example.com",
			configKey:         "play.example.com",
			shouldUseForcedHost: true,
			description:       "Virtual host without port should match hostname config key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := createTestProxyWithForcedHosts(t, map[string]string{
				"forced": "localhost:25566",
				"fallback": "localhost:25567",
			}, map[string][]string{
				tc.configKey: {"forced"},
			}, []string{"fallback"})

			player := &connectedPlayer{
				sessionHandlerDeps: &sessionHandlerDeps{
					proxy:          proxy,
					configProvider: &testConfigProvider{cfg: proxy.cfg},
				},
				virtualHost:  netutil.NewAddr(tc.virtualHost, "tcp"),
				serversToTry: nil,
				tryIndex:     0,
			}

			firstServer := player.nextServerToTry(nil)
			require.NotNil(t, firstServer)

			if tc.shouldUseForcedHost {
				assert.Equal(t, "forced", firstServer.ServerInfo().Name(), 
					"Should use forced host: %s", tc.description)
				assert.Equal(t, []string{"forced"}, player.serversToTry)
			} else {
				assert.Equal(t, "fallback", firstServer.ServerInfo().Name(), 
					"Should use fallback: %s", tc.description)
				assert.Equal(t, []string{"fallback"}, player.serversToTry)
			}
		})
	}
}

// createTestProxyWithForcedHosts creates a test proxy with the given configuration
func createTestProxyWithForcedHosts(t *testing.T, servers map[string]string, forcedHosts map[string][]string, tryList []string) *Proxy {
	cfg := &config.Config{
		Servers:     servers,
		ForcedHosts: forcedHosts,
		Try:         tryList,
		Lite:        liteconfig.Config{Enabled: false}, // Disable lite mode for server registration
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

// testConfigProvider implements the configProvider interface for testing
type testConfigProvider struct {
	cfg *config.Config
}

func (t *testConfigProvider) config() *config.Config {
	return t.cfg
}

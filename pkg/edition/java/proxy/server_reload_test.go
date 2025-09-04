package proxy

import (
	"net"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
)

// TestServerConfigReload tests that servers are properly added and removed
// during config reloads, while preserving API-registered servers.
func TestServerConfigReload(t *testing.T) {
	// Helper to create test proxy
	createProxy := func(t *testing.T, servers map[string]string) *Proxy {
		cfg := &config.Config{
			Bind:       "localhost:25565",
			OnlineMode: false,
			Servers:    servers,
			Lite: liteconfig.Config{
				Enabled: false,
			},
		}

		proxy := &Proxy{
			log:           testr.New(t),
			cfg:           cfg,
			servers:       make(map[string]*registeredServer),
			configServers: make(map[string]bool),
			authenticator: func() auth.Authenticator {
				a, _ := auth.New(auth.Options{})
				return a
			}(),
		}

		err := proxy.init()
		require.NoError(t, err)
		return proxy
	}

	t.Run("AddServerOnReload", func(t *testing.T) {
		// Start with 2 servers
		proxy := createProxy(t, map[string]string{
			"server1": "localhost:25566",
			"server2": "localhost:25567",
		})
		require.Len(t, proxy.Servers(), 2)

		// Reload with 3 servers
		proxy.cfg.Servers = map[string]string{
			"server1": "localhost:25566",
			"server2": "localhost:25567",
			"server3": "localhost:25568",
		}
		err := proxy.init()
		require.NoError(t, err)

		// Should have 3 servers
		require.Len(t, proxy.Servers(), 3)
		require.NotNil(t, proxy.Server("server3"))
	})

	t.Run("RemoveServerOnReload", func(t *testing.T) {
		// Start with 3 servers
		proxy := createProxy(t, map[string]string{
			"server1": "localhost:25566",
			"server2": "localhost:25567",
			"server3": "localhost:25568",
		})
		require.Len(t, proxy.Servers(), 3)

		// Reload with 2 servers (server3 removed)
		proxy.cfg.Servers = map[string]string{
			"server1": "localhost:25566",
			"server2": "localhost:25567",
		}
		err := proxy.init()
		require.NoError(t, err)

		// Should have 2 servers, server3 should be gone
		require.Len(t, proxy.Servers(), 2)
		require.Nil(t, proxy.Server("server3"))
		require.NotNil(t, proxy.Server("server1"))
		require.NotNil(t, proxy.Server("server2"))
	})

	t.Run("PreserveAPIServers", func(t *testing.T) {
		// Start with 2 config servers
		proxy := createProxy(t, map[string]string{
			"server1": "localhost:25566",
			"server2": "localhost:25567",
		})

		// Add an API server (not from config)
		apiAddr, _ := net.ResolveTCPAddr("tcp", "localhost:25569")
		apiServer := NewServerInfo("apiserver", apiAddr)
		_, err := proxy.Register(apiServer)
		require.NoError(t, err)
		require.Len(t, proxy.Servers(), 3)

		// Reload config with only 1 server
		proxy.cfg.Servers = map[string]string{
			"server1": "localhost:25566",
		}
		err = proxy.init()
		require.NoError(t, err)

		// Should have 2 servers: server1 (config) + apiserver (API)
		// server2 should be removed
		require.Len(t, proxy.Servers(), 2)
		require.NotNil(t, proxy.Server("server1"))
		require.NotNil(t, proxy.Server("apiserver"))
		require.Nil(t, proxy.Server("server2"))
	})

	t.Run("UpdateServerAddress", func(t *testing.T) {
		// Start with server1 on port 25566
		proxy := createProxy(t, map[string]string{
			"server1": "localhost:25566",
		})
		server1 := proxy.Server("server1")
		require.NotNil(t, server1)
		require.Equal(t, "localhost:25566", server1.ServerInfo().Addr().String())

		// Reload with server1 on different port
		proxy.cfg.Servers = map[string]string{
			"server1": "localhost:25577",
		}
		err := proxy.init()
		require.NoError(t, err)

		// Server1 should have new address
		server1 = proxy.Server("server1")
		require.NotNil(t, server1)
		require.Equal(t, "localhost:25577", server1.ServerInfo().Addr().String())
	})

	t.Run("CompleteServerReplacement", func(t *testing.T) {
		// Start with servers 1-3
		proxy := createProxy(t, map[string]string{
			"server1": "localhost:25566",
			"server2": "localhost:25567",
			"server3": "localhost:25568",
		})
		require.Len(t, proxy.Servers(), 3)

		// Replace with completely different servers 4-6
		proxy.cfg.Servers = map[string]string{
			"server4": "localhost:25569",
			"server5": "localhost:25570",
			"server6": "localhost:25571",
		}
		err := proxy.init()
		require.NoError(t, err)

		// Should have only the new servers
		require.Len(t, proxy.Servers(), 3)
		require.Nil(t, proxy.Server("server1"))
		require.Nil(t, proxy.Server("server2"))
		require.Nil(t, proxy.Server("server3"))
		require.NotNil(t, proxy.Server("server4"))
		require.NotNil(t, proxy.Server("server5"))
		require.NotNil(t, proxy.Server("server6"))
	})

	t.Run("EmptyConfigPreservesAPIServers", func(t *testing.T) {
		// Start with 1 config server
		proxy := createProxy(t, map[string]string{
			"server1": "localhost:25566",
		})

		// Add an API server
		apiAddr, _ := net.ResolveTCPAddr("tcp", "localhost:25569")
		apiServer := NewServerInfo("apiserver", apiAddr)
		_, err := proxy.Register(apiServer)
		require.NoError(t, err)
		require.Len(t, proxy.Servers(), 2)

		// Reload with empty config
		proxy.cfg.Servers = map[string]string{}
		err = proxy.init()
		require.NoError(t, err)

		// Should only have API server left
		require.Len(t, proxy.Servers(), 1)
		require.NotNil(t, proxy.Server("apiserver"))
		require.Nil(t, proxy.Server("server1"))
	})
}

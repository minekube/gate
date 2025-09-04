package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// TestLoadConfig_MapSharing tests that LoadConfig creates fresh maps
// for each load to avoid state sharing between config reloads.
// This was a bug where removed servers would persist after config reload.
func TestLoadConfig_MapSharing(t *testing.T) {
	// Create a temporary directory for test configs
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")

	// Config with 4 servers
	configWithServer4 := `
config:
  bind: 0.0.0.0:25565
  onlineMode: true
  servers:
    server1: localhost:25566
    server2: localhost:25567
    server3: localhost:25569
    server4: localhost:25570
`

	// Config with server4 commented out (3 servers)
	configWithoutServer4 := `
config:
  bind: 0.0.0.0:25565
  onlineMode: true
  servers:
    server1: localhost:25566
    server2: localhost:25567
    server3: localhost:25569
    # server4: localhost:25570
`

	// Write initial config with server4
	err := os.WriteFile(configPath, []byte(configWithServer4), 0644)
	require.NoError(t, err)

	// Load config first time
	v1 := viper.New()
	v1.SetConfigFile(configPath)
	cfg1, err := LoadConfig(v1)
	require.NoError(t, err)
	require.Len(t, cfg1.Config.Servers, 4, "Should have 4 servers on first load")
	require.Contains(t, cfg1.Config.Servers, "server4", "server4 should exist on first load")

	// Update config file - comment out server4
	err = os.WriteFile(configPath, []byte(configWithoutServer4), 0644)
	require.NoError(t, err)

	// Load config second time with fresh viper (simulating config reload)
	v2 := viper.New()
	v2.SetConfigFile(configPath)
	cfg2, err := LoadConfig(v2)
	require.NoError(t, err)

	// This is the critical test - server4 should NOT persist
	require.Len(t, cfg2.Config.Servers, 3, "Should have 3 servers after server4 is commented out")
	require.NotContains(t, cfg2.Config.Servers, "server4", "server4 should NOT exist after being commented out")

	// Verify the remaining servers are correct
	require.Contains(t, cfg2.Config.Servers, "server1")
	require.Contains(t, cfg2.Config.Servers, "server2")
	require.Contains(t, cfg2.Config.Servers, "server3")
}

// TestLoadConfig_ForcedHostsMapSharing tests that ForcedHosts map is also
// properly isolated between config loads
func TestLoadConfig_ForcedHostsMapSharing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")

	// Config with forced hosts
	configWithForcedHost := `
config:
  bind: 0.0.0.0:25565
  onlineMode: true
  servers:
    server1: localhost:25566
  forcedHosts:
    "example.com": ["server1"]
    "test.com": ["server1"]
`

	// Config with one forced host removed
	configWithoutTestHost := `
config:
  bind: 0.0.0.0:25565
  onlineMode: true
  servers:
    server1: localhost:25566
  forcedHosts:
    "example.com": ["server1"]
    # "test.com": ["server1"]
`

	// Write initial config
	err := os.WriteFile(configPath, []byte(configWithForcedHost), 0644)
	require.NoError(t, err)

	// Load config first time
	v1 := viper.New()
	v1.SetConfigFile(configPath)
	cfg1, err := LoadConfig(v1)
	require.NoError(t, err)
	require.Len(t, cfg1.Config.ForcedHosts, 2, "Should have 2 forced hosts on first load")
	require.Contains(t, cfg1.Config.ForcedHosts, "test.com")

	// Update config file
	err = os.WriteFile(configPath, []byte(configWithoutTestHost), 0644)
	require.NoError(t, err)

	// Load config second time
	v2 := viper.New()
	v2.SetConfigFile(configPath)
	cfg2, err := LoadConfig(v2)
	require.NoError(t, err)

	// Verify forced host was removed
	require.Len(t, cfg2.Config.ForcedHosts, 1, "Should have 1 forced host after removal")
	require.NotContains(t, cfg2.Config.ForcedHosts, "test.com", "test.com should be removed")
	require.Contains(t, cfg2.Config.ForcedHosts, "example.com", "example.com should remain")
}

// TestLoadConfig_IndependentInstances tests that multiple LoadConfig calls
// create truly independent config instances
func TestLoadConfig_IndependentInstances(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yml")

	config := `
config:
  bind: 0.0.0.0:25565
  onlineMode: true
  servers:
    server1: localhost:25566
`

	err := os.WriteFile(configPath, []byte(config), 0644)
	require.NoError(t, err)

	// Load config multiple times
	v1 := viper.New()
	v1.SetConfigFile(configPath)
	cfg1, err := LoadConfig(v1)
	require.NoError(t, err)

	v2 := viper.New()
	v2.SetConfigFile(configPath)
	cfg2, err := LoadConfig(v2)
	require.NoError(t, err)

	// Modify cfg1's servers map
	cfg1.Config.Servers["dynamic"] = "localhost:9999"

	// cfg2 should NOT be affected
	require.NotContains(t, cfg2.Config.Servers, "dynamic", "Config instances should be independent")
	require.Len(t, cfg2.Config.Servers, 1, "cfg2 should still have only 1 server")
}

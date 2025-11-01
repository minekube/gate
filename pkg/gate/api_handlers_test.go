package gate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	config2 "go.minekube.com/gate/pkg/edition/java/lite/config"
	javaconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func TestConfigHandlerImpl_GetStatus(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    func() *config.Config
		setupProxy     func() *proxy.Proxy
		expectedMode   pb.ProxyMode
		expectClassic  bool
		expectLite     bool
	}{
		{
			name: "classic mode status",
			setupConfig: func() *config.Config {
				return createValidTestConfig(false)
			},
			setupProxy: func() *proxy.Proxy {
				return createMockProxy(false)
			},
			expectedMode:  pb.ProxyMode_PROXY_MODE_CLASSIC,
			expectClassic: true,
			expectLite:    false,
		},
		{
			name: "lite mode status",
			setupConfig: func() *config.Config {
				return createValidTestConfig(true)
			},
			setupProxy: func() *proxy.Proxy {
				return createMockProxy(true)
			},
			expectedMode:  pb.ProxyMode_PROXY_MODE_LITE,
			expectClassic: false,
			expectLite:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			proxy := tt.setupProxy()
			handler := NewConfigHandler(&sync.Mutex{}, cfg, event.Nop, proxy, "")

			resp, err := handler.GetStatus(context.Background(), &pb.GetStatusRequest{})

			require.NoError(t, err)
			assert.NotEmpty(t, resp.Version)
			assert.Equal(t, tt.expectedMode, resp.Mode)

			if tt.expectClassic {
				require.NotNil(t, resp.GetClassic())
				assert.GreaterOrEqual(t, resp.GetClassic().Players, int32(0))
				assert.GreaterOrEqual(t, resp.GetClassic().Servers, int32(0))
				assert.Nil(t, resp.GetLite())
			}

			if tt.expectLite {
				require.NotNil(t, resp.GetLite())
				assert.GreaterOrEqual(t, resp.GetLite().Connections, int32(0))
				assert.Equal(t, int32(2), resp.GetLite().Routes) // 2 routes from setup
				assert.Nil(t, resp.GetClassic())
			}
		})
	}
}

func TestConfigHandlerImpl_GetConfig(t *testing.T) {
	cfg := createValidTestConfig(false)
	handler := NewConfigHandler(&sync.Mutex{}, cfg, event.Nop, nil, "")

	resp, err := handler.GetConfig(context.Background(), &pb.GetConfigRequest{})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Payload)

	// Verify the YAML can be unmarshaled back
	var unmarshaled config.Config
	err = yaml.Unmarshal([]byte(resp.Payload), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, "localhost:25577", unmarshaled.Config.Bind)
	assert.Equal(t, javaconfig.ForwardingMode("none"), unmarshaled.Config.Forwarding.Mode)
}

func TestConfigHandlerImpl_ValidateConfig(t *testing.T) {
	tests := []struct {
		name          string
		configYAML    string
		expectError   bool
		expectWarning bool
	}{
		{
			name: "valid config",
			configYAML: `
config:
  bind: "0.0.0.0:25577"
  onlineMode: true
  forwarding:
    mode: "none"
  servers:
    lobby: "localhost:25565"
`,
			expectError:   false,
			expectWarning: true, // Valid configs can still generate warnings
		},
		{
			name: "invalid YAML",
			configYAML: `
config:
  bind: "0.0.0.0:25577"
  onlineMode: true
  servers:
    lobby: "localhost:25565"
  invalid_yaml: [unclosed
`,
			expectError:   true,
			expectWarning: false,
		},
		{
			name: "valid YAML but invalid config",
			configYAML: `
config:
  bind: ""
  onlineMode: true
  forwarding:
    mode: "invalid"
`,
			expectError:   true,
			expectWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewConfigHandler(&sync.Mutex{}, &config.Config{}, event.Nop, nil, "")

			warnings, err := handler.ValidateConfig(context.Background(), &pb.ValidateConfigRequest{
				Config: tt.configYAML,
			})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectWarning {
					assert.NotEmpty(t, warnings)
				} else {
					assert.Empty(t, warnings)
				}
			}
		})
	}
}

func TestConfigHandlerImpl_ApplyConfig(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		persist     bool
		setupFile   bool
		expectError bool
	}{
		{
			name: "apply valid config without persistence",
			configYAML: `
config:
  bind: "0.0.0.0:25577"
  onlineMode: true
  forwarding:
    mode: "none"
  servers:
    lobby: "localhost:25565"
`,
			persist:     false,
			setupFile:   false,
			expectError: false,
		},
		{
			name: "apply valid config with persistence",
			configYAML: `
config:
  bind: "0.0.0.0:25577"
  onlineMode: false
  forwarding:
    mode: "none"
`,
			persist:     true,
			setupFile:   true,
			expectError: false,
		},
		{
			name: "apply invalid config",
			configYAML: `
config:
  bind: ""
  onlineMode: true
`,
			persist:     false,
			setupFile:   false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			mu := &sync.Mutex{}

			var configFilePath string
			if tt.setupFile {
				// Create a temporary config file
				tmpDir := t.TempDir()
				configFilePath = filepath.Join(tmpDir, "config.yaml")
				err := os.WriteFile(configFilePath, []byte("config:\n  bind: \"localhost:25577\"\n"), 0644)
				require.NoError(t, err)
			}

			handler := NewConfigHandler(mu, cfg, event.Nop, nil, configFilePath)

			warnings, err := handler.ApplyConfig(context.Background(), &pb.ApplyConfigRequest{
				Config:  tt.configYAML,
				Persist: tt.persist,
			})

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, warnings) // No warnings when there's an error
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, warnings) // Warnings should be a slice (empty or with items)

				// Verify config was applied in memory
				mu.Lock()
				assert.Equal(t, "0.0.0.0:25577", cfg.Config.Bind)
				mu.Unlock()

				// If persistence was requested and file exists, verify it was written
				if tt.persist && tt.setupFile {
					data, err := os.ReadFile(configFilePath)
					require.NoError(t, err)
					assert.Contains(t, string(data), "0.0.0.0:25577")
				}
			}
		})
	}
}

func TestConfigHandlerImpl_ApplyConfig_PersistenceErrors(t *testing.T) {
	tests := []struct {
		name           string
		configFilePath string
		expectWarning  bool
	}{
		{
			name:           "no config file path",
			configFilePath: "",
			expectWarning:  true,
		},
		{
			name:           "unsupported file extension",
			configFilePath: "/tmp/config.json",
			expectWarning:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			handler := NewConfigHandler(&sync.Mutex{}, cfg, event.Nop, nil, tt.configFilePath)

			configYAML := `
config:
  bind: "0.0.0.0:25577"
  onlineMode: true
  forwarding:
    mode: "none"
`

			warnings, err := handler.ApplyConfig(context.Background(), &pb.ApplyConfigRequest{
				Config:  configYAML,
				Persist: true,
			})

			require.NoError(t, err) // Should not error, but should warn
			if tt.expectWarning {
				assert.NotEmpty(t, warnings)
				// Check if any warning contains persistence failure message
				found := false
				for _, warning := range warnings {
					if strings.Contains(warning, "failed to persist config to disk") {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected to find persistence warning in: %v", warnings)
			}
		})
	}
}

// Helper function to create a valid minimal config for testing
func createValidTestConfig(liteMode bool) *config.Config {
	cfg := &config.Config{}
	// Set required fields to pass validation
	cfg.Config.Bind = "localhost:25577"
	cfg.Config.Forwarding.Mode = "none"

	if liteMode {
		cfg.Config.Lite.Enabled = true
		cfg.Config.Lite.Routes = []config2.Route{
			{
				Host:    []string{"test1.com", "test1.net"},
				Backend: []string{"server1:25565", "server2:25565"},
			},
			{
				Host:    []string{"test2.com"},
				Backend: []string{"server3:25565"},
			},
		}
	} else {
		cfg.Config.Lite.Enabled = false
		cfg.Config.Servers = map[string]string{
			"lobby": "localhost:25565",
			"game":  "localhost:25566",
		}
	}

	return cfg
}

// Helper function to create a mock proxy for testing
func createMockProxy(liteMode bool) *proxy.Proxy {
	// For unit tests, we return nil and the handlers should handle it gracefully
	// Integration tests would use real proxy instances
	return nil
}
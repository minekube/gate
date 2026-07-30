package proxy

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestRequiresRestart(t *testing.T) {
	// Base configuration for comparison
	baseConfig := &config.Config{
		GeyserListenAddr: "localhost:25567",
		UsernameFormat:   ".%s",
		FloodgateKeyPath: "floodgate.pem",
		Managed: &config.ManagedGeyser{
			Enabled: true,
			JarURL:  "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
			ConfigOverrides: map[string]any{
				"debug-mode": false,
				"bedrock": map[string]any{
					"port": 19132,
				},
			},
		},
	}

	tests := []struct {
		name          string
		modifyConfig  func(cfg *config.Config) *config.Config
		shouldRestart bool
		description   string
	}{
		{
			name: "no changes",
			modifyConfig: func(cfg *config.Config) *config.Config {
				// Create a deep copy to ensure no side effects
				return &config.Config{
					GeyserListenAddr: cfg.GeyserListenAddr,
					UsernameFormat:   cfg.UsernameFormat,
					FloodgateKeyPath: cfg.FloodgateKeyPath,
					Managed: &config.ManagedGeyser{
						Enabled: cfg.Managed.Enabled,
						JarURL:  cfg.Managed.JarURL,
						ConfigOverrides: map[string]any{
							"debug-mode": false,
							"bedrock": map[string]any{
								"port": 19132,
							},
						},
					},
				}
			},
			shouldRestart: false,
			description:   "Identical configurations should not trigger restart",
		},
		{
			name: "geyser listen addr change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				modified.GeyserListenAddr = "localhost:25568"
				return &modified
			},
			shouldRestart: true,
			description:   "Geyser listen address change should trigger restart",
		},
		{
			name: "geyser listen addr change to docker mode",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				modified.GeyserListenAddr = "0.0.0.0:25567" // Docker/exposed mode
				return &modified
			},
			shouldRestart: true,
			description:   "Changing to Docker-exposed address should trigger restart",
		},
		{
			name: "username format change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				modified.UsernameFormat = "bedrock_%s"
				return &modified
			},
			shouldRestart: true,
			description:   "Username format change should trigger restart",
		},
		{
			name: "floodgate key path change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				modified.FloodgateKeyPath = "new-key.pem"
				return &modified
			},
			shouldRestart: true,
			description:   "Floodgate key path change should trigger restart",
		},
		{
			name: "backend floodgate enabled change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				modified.BackendFloodgate = config.BackendFloodgate{
					Enabled:        true,
					AllowedServers: []string{"lobby"},
				}
				return &modified
			},
			shouldRestart: true,
			description:   "Enabling backend Floodgate compatibility should trigger restart",
		},
		{
			name: "backend floodgate allowed server change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				modified.BackendFloodgate = config.BackendFloodgate{
					Enabled:        false,
					AllowedServers: []string{"lobby"},
				}
				return &modified
			},
			shouldRestart: true,
			description:   "Changing backend Floodgate allowed servers should trigger restart",
		},
		{
			name: "managed enabled toggle",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.Enabled = false
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Toggling managed mode should trigger restart",
		},
		{
			name: "jar URL change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.JarURL = "https://custom.example.com/geyser.jar"
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "JAR URL change should trigger restart",
		},
		{
			name: "managed engine change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.Engine = config.ManagedEngineJava
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Managed engine change should trigger restart",
		},
		{
			name: "bedrock port change in configOverrides",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.ConfigOverrides = map[string]any{
					"debug-mode": false,
					"bedrock": map[string]any{
						"port": 19133, // Changed port
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Bedrock port change in configOverrides should trigger restart",
		},
		{
			name: "debug mode change in configOverrides",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.ConfigOverrides = map[string]any{
					"debug-mode": true, // Changed debug mode
					"bedrock": map[string]any{
						"port": 19132,
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Debug mode change in configOverrides should trigger restart",
		},
		{
			name: "adding new configOverride",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.ConfigOverrides = map[string]any{
					"debug-mode":  false,
					"max-players": 200, // New override
					"bedrock": map[string]any{
						"port": 19132,
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Adding new configOverride should trigger restart",
		},
		{
			name: "removing configOverride",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.ConfigOverrides = map[string]any{
					// Removed debug-mode
					"bedrock": map[string]any{
						"port": 19132,
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Removing configOverride should trigger restart",
		},
		{
			name: "nested configOverride change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.ConfigOverrides = map[string]any{
					"debug-mode": false,
					"bedrock": map[string]any{
						"port":              19132,
						"compression-level": 8, // Added nested field
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: true,
			description:   "Nested configOverride changes should trigger restart",
		},
		{
			name: "non-critical managed field change",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.DataDir = "/custom/data" // Non-critical field
				// Keep ConfigOverrides identical
				managedCopy.ConfigOverrides = map[string]any{
					"debug-mode": false,
					"bedrock": map[string]any{
						"port": 19132,
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: false,
			description:   "Non-critical managed field changes should not trigger restart",
		},
		{
			name: "auto update change (non-critical)",
			modifyConfig: func(cfg *config.Config) *config.Config {
				modified := *cfg
				managedCopy := *cfg.Managed
				managedCopy.AutoUpdate = !cfg.Managed.AutoUpdate // Toggle auto update
				// Keep ConfigOverrides identical
				managedCopy.ConfigOverrides = map[string]any{
					"debug-mode": false,
					"bedrock": map[string]any{
						"port": 19132,
					},
				}
				modified.Managed = &managedCopy
				return &modified
			},
			shouldRestart: false,
			description:   "AutoUpdate changes should not trigger restart (affects next startup)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifiedConfig := tt.modifyConfig(baseConfig)
			result := requiresRestart(baseConfig, modifiedConfig)

			if result != tt.shouldRestart {
				t.Errorf("requiresRestart() = %v, expected %v. %s",
					result, tt.shouldRestart, tt.description)
			}
		})
	}
}

func TestStartClearsBackendHandshakeHookWhenIntegrationStartFails(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{0x24}, 16), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	javaProxy, err := jproxy.New(jproxy.Options{
		Config:   &jconfig.DefaultConfig,
		EventMgr: event.Nop,
	})
	if err != nil {
		t.Fatalf("jproxy.New() error = %v", err)
	}
	p, err := New(Options{
		Config: &config.Config{
			FloodgateKeyPath: keyPath,
			GeyserListenAddr: "127.0.0.1",
			BackendFloodgate: config.BackendFloodgate{
				Enabled:        true,
				AllowedServers: []string{"lobby"},
			},
		},
		JavaProxy: javaProxy,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := p.Start(context.Background()); err == nil {
		t.Fatal("Start() returned nil error for invalid listen address")
	}
	if backendHandshakeAddresserRegistered(javaProxy) {
		t.Fatal("backend handshake addresser remains registered after failed start")
	}
}

func TestConfigUpdateNilCurrentStopsIntegrationAndClearsHook(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{0x24}, 16), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	javaProxy, err := jproxy.New(jproxy.Options{
		Config:   &jconfig.DefaultConfig,
		EventMgr: event.Nop,
	})
	if err != nil {
		t.Fatalf("jproxy.New() error = %v", err)
	}
	cfg := &config.Config{
		FloodgateKeyPath: keyPath,
		GeyserListenAddr: "127.0.0.1:0",
		BackendFloodgate: config.BackendFloodgate{
			Enabled:        true,
			AllowedServers: []string{"lobby"},
		},
	}
	integration, err := geyser.NewIntegration(context.Background(), javaProxy, cfg)
	if err != nil {
		t.Fatalf("geyser.NewIntegration() error = %v", err)
	}
	if !backendHandshakeAddresserRegistered(javaProxy) {
		t.Fatal("backend handshake addresser was not registered")
	}

	p := &Proxy{
		config:            cfg,
		javaProxy:         javaProxy,
		geyserIntegration: integration,
	}
	p.handleConfigUpdate(context.Background(), &bedrockConfigUpdateEvent{
		PrevConfig: cfg,
		Config:     nil,
	})

	if p.geyserIntegration != nil {
		t.Fatal("geyserIntegration remains set after disabled Bedrock config update")
	}
	if backendHandshakeAddresserRegistered(javaProxy) {
		t.Fatal("backend handshake addresser remains registered after disabled Bedrock config update")
	}
}

func backendHandshakeAddresserRegistered(p *jproxy.Proxy) bool {
	field := reflect.ValueOf(p).Elem().FieldByName("backendHandshakeAddresser")
	return !field.IsNil()
}

// TestRequiresRestart_EdgeCases tests edge cases for the restart logic
func TestRequiresRestart_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		prev          *config.Config
		curr          *config.Config
		shouldRestart bool
		description   string
	}{
		{
			name: "nil managed to enabled managed",
			prev: &config.Config{
				GeyserListenAddr: "localhost:25567",
				Managed:          nil,
			},
			curr: &config.Config{
				GeyserListenAddr: "localhost:25567",
				Managed: &config.ManagedGeyser{
					Enabled: true,
				},
			},
			shouldRestart: true,
			description:   "Adding managed config should trigger restart",
		},
		{
			name: "enabled managed to nil managed",
			prev: &config.Config{
				GeyserListenAddr: "localhost:25567",
				Managed: &config.ManagedGeyser{
					Enabled: true,
				},
			},
			curr: &config.Config{
				GeyserListenAddr: "localhost:25567",
				Managed:          nil,
			},
			shouldRestart: true,
			description:   "Removing managed config should trigger restart",
		},
		{
			name: "nil config overrides to empty config overrides",
			prev: &config.Config{
				GeyserListenAddr: "localhost:25567",
				Managed: &config.ManagedGeyser{
					Enabled:         true,
					ConfigOverrides: nil,
				},
			},
			curr: &config.Config{
				GeyserListenAddr: "localhost:25567",
				Managed: &config.ManagedGeyser{
					Enabled:         true,
					ConfigOverrides: map[string]any{},
				},
			},
			shouldRestart: true, // reflect.DeepEqual treats nil != empty map
			description:   "nil to empty configOverrides should trigger restart (DeepEqual behavior)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := requiresRestart(tt.prev, tt.curr)
			if result != tt.shouldRestart {
				t.Errorf("requiresRestart() = %v, expected %v. %s",
					result, tt.shouldRestart, tt.description)
			}
		})
	}
}

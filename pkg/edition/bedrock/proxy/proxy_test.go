package proxy

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
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

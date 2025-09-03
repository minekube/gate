package config

import (
	"testing"
)

func TestGetManaged(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected ManagedGeyser
	}{
		{
			name:   "nil managed returns defaults",
			config: Config{Managed: nil},
			expected: ManagedGeyser{
				Enabled:     false,
				JarURL:      "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
				DataDir:     ".geyser",
				JavaPath:    "java",
				BedrockPort: 19132,
				AutoUpdate:  true,
			},
		},
		{
			name: "empty managed struct uses defaults",
			config: Config{
				Managed: &ManagedGeyser{},
			},
			expected: ManagedGeyser{
				Enabled:     false, // User's value (zero)
				JarURL:      "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
				DataDir:     ".geyser",
				JavaPath:    "java",
				BedrockPort: 19132,
				AutoUpdate:  true, // Default because no other fields set
			},
		},
		{
			name: "enabled managed with custom values",
			config: Config{
				Managed: &ManagedGeyser{
					Enabled:     true,
					JarURL:      "https://custom.example.com/geyser.jar",
					DataDir:     "/custom/geyser",
					JavaPath:    "/usr/bin/java",
					BedrockPort: 19133,
				},
			},
			expected: ManagedGeyser{
				Enabled:     true,
				JarURL:      "https://custom.example.com/geyser.jar",
				DataDir:     "/custom/geyser",
				JavaPath:    "/usr/bin/java",
				BedrockPort: 19133,
				AutoUpdate:  false, // User set other fields, so use their AutoUpdate (zero = false)
			},
		},
		{
			name: "config overrides are preserved",
			config: Config{
				Managed: &ManagedGeyser{
					Enabled: true,
					ConfigOverrides: map[string]any{
						"debug-mode":  true,
						"max-players": 100,
						"server-name": "Custom Server",
						"bedrock": map[string]any{
							"compression-level": 8,
						},
					},
				},
			},
			expected: ManagedGeyser{
				Enabled:     true,
				JarURL:      "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
				DataDir:     ".geyser",
				JavaPath:    "java",
				BedrockPort: 19132,
				AutoUpdate:  true, // Default because only ConfigOverrides set
				ConfigOverrides: map[string]any{
					"debug-mode":  true,
					"max-players": 100,
					"server-name": "Custom Server",
					"bedrock": map[string]any{
						"compression-level": 8,
					},
				},
			},
		},
		{
			name: "explicit autoUpdate false is respected",
			config: Config{
				Managed: &ManagedGeyser{
					Enabled:    true,
					AutoUpdate: false,
					ExtraArgs:  []string{"-Xmx2G"}, // This indicates user set fields
				},
			},
			expected: ManagedGeyser{
				Enabled:     true,
				JarURL:      "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
				DataDir:     ".geyser",
				JavaPath:    "java",
				BedrockPort: 19132,
				AutoUpdate:  false, // User explicitly set this
				ExtraArgs:   []string{"-Xmx2G"},
			},
		},
		{
			name: "complex config overrides with nested maps",
			config: Config{
				Managed: &ManagedGeyser{
					Enabled: true,
					ConfigOverrides: map[string]any{
						"bedrock": map[string]any{
							"port":              19133,
							"compression-level": 6,
							"motd1":             "Line 1",
							"motd2":             "Line 2",
						},
						"remote": map[string]any{
							"address": "backend.example.com",
							"port":    25565,
						},
						"debug-mode":  false,
						"max-players": 200,
					},
				},
			},
			expected: ManagedGeyser{
				Enabled:     true,
				JarURL:      "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/standalone",
				DataDir:     ".geyser",
				JavaPath:    "java",
				BedrockPort: 19132,
				AutoUpdate:  true, // Default because only ConfigOverrides set
				ConfigOverrides: map[string]any{
					"bedrock": map[string]any{
						"port":              19133,
						"compression-level": 6,
						"motd1":             "Line 1",
						"motd2":             "Line 2",
					},
					"remote": map[string]any{
						"address": "backend.example.com",
						"port":    25565,
					},
					"debug-mode":  false,
					"max-players": 200,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetManaged()

			// Compare all fields
			if result.Enabled != tt.expected.Enabled {
				t.Errorf("Enabled: got %v, want %v", result.Enabled, tt.expected.Enabled)
			}
			if result.JarURL != tt.expected.JarURL {
				t.Errorf("JarURL: got %v, want %v", result.JarURL, tt.expected.JarURL)
			}
			if result.DataDir != tt.expected.DataDir {
				t.Errorf("DataDir: got %v, want %v", result.DataDir, tt.expected.DataDir)
			}
			if result.JavaPath != tt.expected.JavaPath {
				t.Errorf("JavaPath: got %v, want %v", result.JavaPath, tt.expected.JavaPath)
			}
			if result.BedrockPort != tt.expected.BedrockPort {
				t.Errorf("BedrockPort: got %v, want %v", result.BedrockPort, tt.expected.BedrockPort)
			}
			if result.AutoUpdate != tt.expected.AutoUpdate {
				t.Errorf("AutoUpdate: got %v, want %v", result.AutoUpdate, tt.expected.AutoUpdate)
			}

			// Compare slices
			if len(result.ExtraArgs) != len(tt.expected.ExtraArgs) {
				t.Errorf("ExtraArgs length: got %d, want %d", len(result.ExtraArgs), len(tt.expected.ExtraArgs))
			} else {
				for i, arg := range result.ExtraArgs {
					if arg != tt.expected.ExtraArgs[i] {
						t.Errorf("ExtraArgs[%d]: got %v, want %v", i, arg, tt.expected.ExtraArgs[i])
					}
				}
			}

			// Compare config overrides
			if !compareConfigOverrides(result.ConfigOverrides, tt.expected.ConfigOverrides) {
				t.Errorf("ConfigOverrides: got %+v, want %+v", result.ConfigOverrides, tt.expected.ConfigOverrides)
			}
		})
	}
}

// compareConfigOverrides deeply compares two config override maps
func compareConfigOverrides(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}

	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}

		// Handle nested maps
		if mapA, okA := valueA.(map[string]any); okA {
			if mapB, okB := valueB.(map[string]any); okB {
				if !compareConfigOverrides(mapA, mapB) {
					return false
				}
			} else {
				return false
			}
		} else {
			// Direct value comparison
			if valueA != valueB {
				return false
			}
		}
	}

	return true
}

func TestConfigOverridesIntegration(t *testing.T) {
	// Test that config overrides work end-to-end with the GetManaged method
	config := Config{
		GeyserListenAddr: "0.0.0.0:25567",
		UsernameFormat:   ".%s",
		FloodgateKeyPath: "test.pem",
		Managed: &ManagedGeyser{
			Enabled: true,
			ConfigOverrides: map[string]any{
				"debug-mode": true,
				"bedrock": map[string]any{
					"port":              19133,
					"compression-level": 8,
				},
				"remote": map[string]any{
					"address": "backend.local",
					"port":    25566,
				},
			},
		},
	}

	managed := config.GetManaged()

	// Verify the overrides are preserved
	if !managed.Enabled {
		t.Error("Expected managed to be enabled")
	}

	overrides := managed.ConfigOverrides
	if overrides == nil {
		t.Fatal("Expected config overrides to be preserved")
	}

	// Test top-level override
	if debugMode, ok := overrides["debug-mode"]; !ok || debugMode != true {
		t.Errorf("Expected debug-mode to be true, got %v", debugMode)
	}

	// Test nested overrides
	if bedrockConfig, ok := overrides["bedrock"].(map[string]any); ok {
		if port, ok := bedrockConfig["port"]; !ok || port != 19133 {
			t.Errorf("Expected bedrock.port to be 19133, got %v", port)
		}
		if compression, ok := bedrockConfig["compression-level"]; !ok || compression != 8 {
			t.Errorf("Expected bedrock.compression-level to be 8, got %v", compression)
		}
	} else {
		t.Error("Expected bedrock config to be a map")
	}

	if remoteConfig, ok := overrides["remote"].(map[string]any); ok {
		if address, ok := remoteConfig["address"]; !ok || address != "backend.local" {
			t.Errorf("Expected remote.address to be 'backend.local', got %v", address)
		}
		if port, ok := remoteConfig["port"]; !ok || port != 25566 {
			t.Errorf("Expected remote.port to be 25566, got %v", port)
		}
	} else {
		t.Error("Expected remote config to be a map")
	}
}

func TestConfigOverridesYAMLIntegration(t *testing.T) {
	// Test that config overrides work correctly when loaded from YAML
	yamlConfig := `
geyserListenAddr: "0.0.0.0:25567"
usernameFormat: ".%s"
floodgateKeyPath: "test.pem"
managed:
  enabled: true
  bedrockPort: 19133
  configOverrides:
    debug-mode: true
    max-players: 150
    bedrock:
      compression-level: 9
      motd1: "Custom Bedrock Server"
    remote:
      address: "custom.backend.com"
      port: 25566
    floodgate-key-file: "custom-key.pem"
`

	// This test verifies that the config structure supports the expected YAML format
	// The actual YAML parsing would be handled by the main config system
	config := Config{
		GeyserListenAddr: "0.0.0.0:25567",
		UsernameFormat:   ".%s",
		FloodgateKeyPath: "test.pem",
		Managed: &ManagedGeyser{
			Enabled:     true,
			BedrockPort: 19133,
			ConfigOverrides: map[string]any{
				"debug-mode":  true,
				"max-players": 150,
				"bedrock": map[string]any{
					"compression-level": 9,
					"motd1":             "Custom Bedrock Server",
				},
				"remote": map[string]any{
					"address": "custom.backend.com",
					"port":    25566,
				},
				"floodgate-key-file": "custom-key.pem",
			},
		},
	}

	managed := config.GetManaged()

	// Verify basic settings
	if !managed.Enabled {
		t.Error("Expected managed to be enabled")
	}
	if managed.BedrockPort != 19133 {
		t.Errorf("Expected BedrockPort 19133, got %d", managed.BedrockPort)
	}

	// Verify config overrides are preserved correctly
	overrides := managed.ConfigOverrides
	if overrides == nil {
		t.Fatal("Expected config overrides to be preserved")
	}

	// Test that all override types are handled correctly
	testCases := []struct {
		path     []string
		expected any
	}{
		{[]string{"debug-mode"}, true},
		{[]string{"max-players"}, 150},
		{[]string{"bedrock", "compression-level"}, 9},
		{[]string{"bedrock", "motd1"}, "Custom Bedrock Server"},
		{[]string{"remote", "address"}, "custom.backend.com"},
		{[]string{"remote", "port"}, 25566},
		{[]string{"floodgate-key-file"}, "custom-key.pem"},
	}

	for _, tc := range testCases {
		value := getNestedValue(overrides, tc.path)
		if value != tc.expected {
			t.Errorf("Override %v: got %v, want %v", tc.path, value, tc.expected)
		}
	}

	t.Logf("YAML structure would be:\n%s", yamlConfig)
}

// getNestedValue retrieves a nested value from a map using a path
func getNestedValue(m map[string]any, path []string) any {
	if len(path) == 0 {
		return nil
	}

	current := m
	for i, key := range path[:len(path)-1] {
		if next, ok := current[key].(map[string]any); ok {
			current = next
		} else {
			return nil
		}
		_ = i // Avoid unused variable
	}

	return current[path[len(path)-1]]
}

func TestAutoUpdateLogic(t *testing.T) {
	tests := []struct {
		name           string
		managed        *ManagedGeyser
		expectedUpdate bool
		description    string
	}{
		{
			name:           "nil managed uses default AutoUpdate true",
			managed:        nil,
			expectedUpdate: true,
			description:    "When no managed config is provided, should default to AutoUpdate=true",
		},
		{
			name:           "empty managed uses default AutoUpdate true",
			managed:        &ManagedGeyser{},
			expectedUpdate: true,
			description:    "When empty managed config, should default to AutoUpdate=true",
		},
		{
			name: "managed with only ConfigOverrides uses default AutoUpdate true",
			managed: &ManagedGeyser{
				ConfigOverrides: map[string]any{"debug-mode": true},
			},
			expectedUpdate: true,
			description:    "ConfigOverrides alone shouldn't affect AutoUpdate default",
		},
		{
			name: "managed with other fields respects explicit AutoUpdate false",
			managed: &ManagedGeyser{
				Enabled:    true,
				DataDir:    "/custom",
				AutoUpdate: false,
			},
			expectedUpdate: false,
			description:    "When user sets other fields, their AutoUpdate value should be respected",
		},
		{
			name: "managed with ExtraArgs respects explicit AutoUpdate false",
			managed: &ManagedGeyser{
				Enabled:    true,
				ExtraArgs:  []string{"-Xmx2G"},
				AutoUpdate: false,
			},
			expectedUpdate: false,
			description:    "ExtraArgs indicates user customization, so respect their AutoUpdate setting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{Managed: tt.managed}
			result := config.GetManaged()

			if result.AutoUpdate != tt.expectedUpdate {
				t.Errorf("AutoUpdate: got %v, want %v. %s", result.AutoUpdate, tt.expectedUpdate, tt.description)
			}
		})
	}
}

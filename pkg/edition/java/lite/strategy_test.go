package lite

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
)

// Note: Strategy tests are limited because they require actual network connectivity
// The checkBackend function attempts to dial backends, so we can't test with fake addresses
func TestStrategyValidation(t *testing.T) {
	t.Log("Strategy functions require actual network connectivity to test properly")
	t.Log("Focusing on configuration validation and basic function structure")

	// Test that strategy constants are defined correctly
	strategies := []config.Strategy{
		config.StrategySequential,
		config.StrategyRandom,
		config.StrategyRoundRobin,
		config.StrategyLeastConnections,
		config.StrategyLowestLatency,
	}

	expectedStrategies := []config.Strategy{"sequential", "random", "round-robin", "least-connections", "lowest-latency"}
	assert.Equal(t, expectedStrategies, strategies, "Strategy constants should match expected values")
}

func TestLeastConnectionsCounterIncrement(t *testing.T) {
	// Test the atomic counter increment/decrement logic directly
	counter := &atomic.Uint32{}

	// Initial count should be 0
	assert.Equal(t, uint32(0), counter.Load(), "Initial count should be 0")

	// Simulate connection (increment counter)
	counter.Add(1)
	assert.Equal(t, uint32(1), counter.Load(), "Count should be 1 after increment")

	// Simulate disconnection (decrement counter using the same logic as the code)
	counter.Add(^uint32(0)) // This is equivalent to subtracting 1 (used in defer statement)
	assert.Equal(t, uint32(0), counter.Load(), "Count should be 0 after decrement")

	// Test multiple increments/decrements
	counter.Add(3) // Add 3 connections
	assert.Equal(t, uint32(3), counter.Load(), "Count should be 3")

	counter.Add(^uint32(0)) // Remove 1 connection
	assert.Equal(t, uint32(2), counter.Load(), "Count should be 2 after one disconnect")

	counter.Add(^uint32(0)) // Remove another connection
	counter.Add(^uint32(0)) // Remove another connection
	assert.Equal(t, uint32(0), counter.Load(), "Count should be 0 after all disconnections")
}

func TestConfigValidation_InvalidStrategy(t *testing.T) {
	cfg := config.Config{
		Routes: []config.Route{
			{
				Host:     []string{"test.com"},
				Backend:  []string{"server:25565"},
				Strategy: "invalid-strategy",
			},
		},
	}

	warns, errs := cfg.Validate()
	assert.Empty(t, warns, "Should have no warnings")
	assert.Len(t, errs, 1, "Should have validation error for invalid strategy")
	assert.Contains(t, errs[0].Error(), "invalid strategy 'invalid-strategy'", "Should mention invalid strategy")
}

func TestConfigValidation_ValidStrategies(t *testing.T) {
	validStrategies := []config.Strategy{
		config.StrategySequential,
		config.StrategyRandom,
		config.StrategyRoundRobin,
		config.StrategyLeastConnections,
		config.StrategyLowestLatency,
		"", // Empty should be valid (defaults to sequential)
	}

	for _, strategy := range validStrategies {
		t.Run("strategy_"+string(strategy), func(t *testing.T) {
			cfg := config.Config{
				Routes: []config.Route{
					{
						Host:     []string{"test.com"},
						Backend:  []string{"server:25565"},
						Strategy: strategy,
					},
				},
			}

			warns, errs := cfg.Validate()
			assert.Empty(t, warns, "Should have no warnings for valid strategy")
			assert.Empty(t, errs, "Should have no errors for valid strategy: %s", strategy)
		})
	}
}

func TestConfigValidation_ParameterWarnings(t *testing.T) {
	tests := []struct {
		name          string
		route         config.Route
		expectWarns   bool
		expectWarnMsg string
	}{
		{
			name: "valid parameter usage",
			route: config.Route{
				Host:    []string{"*.domain.com"},
				Backend: []string{"$1.servers.svc:25565"},
			},
			expectWarns: false,
		},
		{
			name: "parameter without wildcards",
			route: config.Route{
				Host:    []string{"example.com"},
				Backend: []string{"$1.servers.svc:25565"},
			},
			expectWarns:   true,
			expectWarnMsg: "has 0 wildcard(s) but backend",
		},
		{
			name: "parameter index exceeds wildcards",
			route: config.Route{
				Host:    []string{"*.domain.com"},
				Backend: []string{"$2.servers.svc:25565"},
			},
			expectWarns:   true,
			expectWarnMsg: "uses parameter $2",
		},
		{
			name: "multiple parameters some valid some invalid",
			route: config.Route{
				Host:    []string{"*.domain.com"},
				Backend: []string{"$1-$2.servers.svc:25565"},
			},
			expectWarns:   true,
			expectWarnMsg: "uses parameter $2",
		},
		{
			name: "multiple wildcards with valid parameters",
			route: config.Route{
				Host:    []string{"*.*.domain.com"},
				Backend: []string{"$1-$2.servers.svc:25565"},
			},
			expectWarns: false,
		},
		{
			name: "parameter index too high",
			route: config.Route{
				Host:    []string{"*.*.domain.com"},
				Backend: []string{"$1-$2-$3.servers.svc:25565"},
			},
			expectWarns:   true,
			expectWarnMsg: "uses parameter $3",
		},
		{
			name: "question mark wildcard with parameter",
			route: config.Route{
				Host:    []string{"?.domain.com"},
				Backend: []string{"$1.servers.svc:25565"},
			},
			expectWarns: false,
		},
		{
			name: "mixed wildcards with parameters",
			route: config.Route{
				Host:    []string{"*.example.*"},
				Backend: []string{"$1-$2.servers.svc:25565"},
			},
			expectWarns: false,
		},
		{
			name: "no parameters in backend",
			route: config.Route{
				Host:    []string{"*.domain.com"},
				Backend: []string{"static.servers.svc:25565"},
			},
			expectWarns: false,
		},
		{
			name: "multiple hosts with parameters",
			route: config.Route{
				Host:    []string{"*.domain.com", "*.example.com"},
				Backend: []string{"$1.servers.svc:25565"},
			},
			expectWarns: false, // Both hosts have wildcards
		},
		{
			name: "multiple hosts one without wildcards",
			route: config.Route{
				Host:    []string{"example.com", "*.domain.com"},
				Backend: []string{"$1.servers.svc:25565"},
			},
			expectWarns:   true,
			expectWarnMsg: "has 0 wildcard(s) but backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{
				Routes: []config.Route{tt.route},
			}

			warns, errs := cfg.Validate()
			assert.Empty(t, errs, "Should have no errors")

			if tt.expectWarns {
				assert.NotEmpty(t, warns, "Should have warnings")
				if tt.expectWarnMsg != "" {
					found := false
					for _, warn := range warns {
						if assert.Contains(t, warn.Error(), tt.expectWarnMsg, "Warning should contain expected message") {
							found = true
							break
						}
					}
					assert.True(t, found, "Should find warning with expected message")
				}
			} else {
				assert.Empty(t, warns, "Should have no warnings")
			}
		})
	}
}

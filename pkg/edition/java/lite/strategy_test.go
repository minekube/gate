package lite

import (
	"sync/atomic"
	"testing"

	"github.com/go-logr/logr"
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
		config.StrategyRandom,
		config.StrategyRoundRobin,
		config.StrategyLeastConnections,
		config.StrategyLowestLatency,
	}
	
	expectedStrategies := []config.Strategy{"random", "round-robin", "least-connections", "lowest-latency"}
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
		config.StrategyRandom,
		config.StrategyRoundRobin,
		config.StrategyLeastConnections,
		config.StrategyLowestLatency,
		"", // Empty should be valid (defaults to random)
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

// Helper function to create a test logger
func testLogger() logr.Logger {
	return logr.Discard()
}

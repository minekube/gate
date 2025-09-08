package lite

import (
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
)

// TestDefaultBehavior_NoStrategy verifies that when no strategy is configured,
// backends are tried in sequential order (not random)
func TestDefaultBehavior_NoStrategy(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)

	// Create route with NO strategy defined (empty string)
	route := &config.Route{
		Host:     []string{"test.example.com"},
		Backend:  []string{"server1:25565", "server2:25565", "server3:25565"},
		Strategy: "", // No strategy defined - should default to sequential
	}

	// Test that empty strategy defaults to sequential behavior
	backends := []string{"server1:25565", "server2:25565", "server3:25565"}

	// Should behave the same as explicit sequential strategy
	backend1, _, ok1 := sm.GetNextBackend(log, route, "test.example.com", backends)
	require.True(t, ok1, "Should return a backend")

	// Explicit sequential for comparison
	sequentialRoute := &config.Route{
		Strategy: config.StrategySequential,
	}
	backend2, _, ok2 := sm.GetNextBackend(log, sequentialRoute, "test.example.com", backends)
	require.True(t, ok2, "Should return a backend")

	// Both should return the same backend (first one)
	assert.Equal(t, backend1, backend2, "Empty strategy should behave same as explicit sequential")
	assert.Equal(t, "server1:25565", backend1, "Should return first backend for default strategy")
}

// TestDefaultBehavior_ExplicitRandom verifies that explicit random strategy works differently
func TestDefaultBehavior_ExplicitRandom(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)

	// Create route with explicit random strategy
	route := &config.Route{
		Host:     []string{"test.example.com"},
		Backend:  []string{"server1:25565", "server2:25565", "server3:25565"},
		Strategy: config.StrategyRandom, // Explicit random
	}

	// Test multiple selections to verify randomness
	selections := make(map[string]int)
	for i := 0; i < 50; i++ {
		backend, _, ok := sm.GetNextBackend(log, route, "test.example.com", route.Backend)
		require.True(t, ok, "Should return a backend")
		selections[backend]++
	}

	// All backends should have been selected (with random, not just first one)
	for _, expectedBackend := range route.Backend {
		assert.Greater(t, selections[expectedBackend], 0,
			"Random strategy should select backend %s at least once", expectedBackend)
	}

	// Should have selected multiple different backends (not just the first one repeatedly)
	assert.Greater(t, len(selections), 1, "Random strategy should select multiple backends")
}

// TestSequentialVsRandomBehavior compares sequential and random strategies
func TestSequentialVsRandomBehavior(t *testing.T) {
	t.Run("sequential always returns first backend", func(t *testing.T) {
		sm := NewStrategyManager()
		log := testr.New(t)
		backends := []string{"a:25565", "b:25565", "c:25565"}

		// Sequential should always return first backend from any list
		for i := 0; i < 10; i++ {
			backend, _, ok := sm.sequentialNextBackend(log, backends)
			require.True(t, ok, "Should return a backend")
			assert.Equal(t, "a:25565", backend, "Sequential should always return first backend")
		}

		// Test with different backend lists
		backends2 := []string{"x:25565", "y:25565"}
		backend, _, ok := sm.sequentialNextBackend(log, backends2)
		require.True(t, ok, "Should return a backend")
		assert.Equal(t, "x:25565", backend, "Should return first backend of different list")
	})

	t.Run("random produces varied results", func(t *testing.T) {
		sm := NewStrategyManager()
		log := testr.New(t)
		backends := []string{"a:25565", "b:25565", "c:25565"}

		// Random should eventually select all backends
		selections := make(map[string]bool)
		for i := 0; i < 100; i++ {
			backend, _, _ := sm.randomNextBackend(log, backends)
			selections[backend] = true
		}

		// Should have selected multiple backends (high probability)
		assert.Greater(t, len(selections), 1, "Random should select multiple backends over 100 attempts")
	})
}

// Helper function to compare slices
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

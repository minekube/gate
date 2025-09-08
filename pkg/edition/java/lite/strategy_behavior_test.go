package lite

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
)

// Test each strategy behavior individually

func TestRandomStrategy(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	backends := []string{"server1:25565", "server2:25565", "server3:25565"}

	// Random should eventually select different backends
	selections := make(map[string]int)
	for i := 0; i < 100; i++ {
		backend, _, ok := sm.randomNextBackend(log, backends)
		require.True(t, ok, "Should return a backend")
		require.Contains(t, backends, backend, "Should return one of the configured backends")
		selections[backend]++
	}

	// All backends should have been selected at least once with 100 attempts
	for _, backend := range backends {
		assert.Greater(t, selections[backend], 0, "Backend %s should be selected at least once", backend)
	}
	
	// Distribution shouldn't be completely even (it's random) but shouldn't be all one backend
	assert.Greater(t, len(selections), 1, "Should have selected multiple different backends")
}

func TestRoundRobinStrategy(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	backends := []string{"server1:25565", "server2:25565", "server3:25565"}
	routeHost := "test.example.com"

	// Round-robin should cycle through backends in order
	var selections []string
	for i := 0; i < 9; i++ { // 3 full cycles
		backend, _, ok := sm.roundRobinNextBackend(log, routeHost, backends)
		require.True(t, ok, "Should return a backend")
		require.Contains(t, backends, backend, "Should return one of the configured backends")
		selections = append(selections, backend)
	}

	// Should cycle through in order: 1,2,3,1,2,3,1,2,3
	expected := []string{
		"server1:25565", "server2:25565", "server3:25565", // First cycle
		"server1:25565", "server2:25565", "server3:25565", // Second cycle
		"server1:25565", "server2:25565", "server3:25565", // Third cycle
	}
	
	assert.Equal(t, expected, selections, "Round-robin should cycle through backends in order")
}

func TestRoundRobinStrategy_DifferentRoutes(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	backends := []string{"server1:25565", "server2:25565"}
	
	// Different routes should have independent round-robin state
	route1 := "route1.example.com"
	route2 := "route2.example.com"

	// Get first backend from each route
	backend1_route1, _, _ := sm.roundRobinNextBackend(log, route1, backends)
	backend1_route2, _, _ := sm.roundRobinNextBackend(log, route2, backends)
	
	// Both should start with the first backend
	assert.Equal(t, "server1:25565", backend1_route1, "Route1 should start with first backend")
	assert.Equal(t, "server1:25565", backend1_route2, "Route2 should start with first backend")
	
	// Get second backend from route1
	backend2_route1, _, _ := sm.roundRobinNextBackend(log, route1, backends)
	assert.Equal(t, "server2:25565", backend2_route1, "Route1 should move to second backend")
	
	// Route2 should still be independent
	backend2_route2, _, _ := sm.roundRobinNextBackend(log, route2, backends)
	assert.Equal(t, "server2:25565", backend2_route2, "Route2 should also move to second backend")
}

func TestLeastConnectionsStrategy(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	backends := []string{"server1:25565", "server2:25565", "server3:25565"}

	// Initially, all backends have 0 connections, should pick first one
	backend1, _, ok := sm.leastConnectionsNextBackend(log, backends)
	require.True(t, ok, "Should return a backend")
	assert.Equal(t, "server1:25565", backend1, "Should pick first backend when all have 0 connections")

	// Simulate connections on server1
	decrementFunc1 := sm.IncrementConnection("server1:25565")
	decrementFunc2 := sm.IncrementConnection("server1:25565")

	// Now should pick server2 (0 connections) instead of server1 (2 connections)
	backend2, _, ok := sm.leastConnectionsNextBackend(log, backends)
	require.True(t, ok, "Should return a backend")
	assert.Equal(t, "server2:25565", backend2, "Should pick server2 with fewer connections")

	// Add connection to server2
	decrementFunc3 := sm.IncrementConnection("server2:25565")

	// Now should pick server3 (0 connections)
	backend3, _, ok := sm.leastConnectionsNextBackend(log, backends)
	require.True(t, ok, "Should return a backend")
	assert.Equal(t, "server3:25565", backend3, "Should pick server3 with fewest connections")

	// Clean up
	decrementFunc1()
	decrementFunc2()
	decrementFunc3()
}

func TestLowestLatencyStrategy(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	backends := []string{"fast:25565", "slow:25565", "medium:25565"}

	// With no latency data, should pick first backend
	backend1, _, ok := sm.lowestLatencyNextBackend(log, backends)
	require.True(t, ok, "Should return a backend")
	assert.Equal(t, "fast:25565", backend1, "Should pick first backend when no latency data")

	// Record latencies
	sm.RecordLatency("fast:25565", 10*time.Millisecond)
	sm.RecordLatency("slow:25565", 100*time.Millisecond)  
	sm.RecordLatency("medium:25565", 50*time.Millisecond)

	// Should pick the fastest backend
	backend2, _, ok := sm.lowestLatencyNextBackend(log, backends)
	require.True(t, ok, "Should return a backend")
	assert.Equal(t, "fast:25565", backend2, "Should pick fastest backend")

	// Try multiple times to ensure consistency
	for i := 0; i < 10; i++ {
		backend, _, ok := sm.lowestLatencyNextBackend(log, backends)
		require.True(t, ok, "Should return a backend")
		assert.Equal(t, "fast:25565", backend, "Should consistently pick fastest backend")
	}
}

func TestStrategyWithEmptyBackends(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	emptyBackends := []string{}

	tests := []struct {
		name     string
		testFunc func(logr.Logger, []string) (string, logr.Logger, bool)
	}{
		{"random", sm.randomNextBackend},
		{"roundRobin", func(log logr.Logger, backends []string) (string, logr.Logger, bool) {
			return sm.roundRobinNextBackend(log, "test.host", backends)
		}},
		{"leastConnections", sm.leastConnectionsNextBackend},
		{"lowestLatency", sm.lowestLatencyNextBackend},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, ok := tt.testFunc(log, emptyBackends)
			assert.False(t, ok, "Strategy should return false for empty backend list")
		})
	}
}

func TestStrategyWithSingleBackend(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	singleBackend := []string{"only:25565"}

	tests := []struct {
		name     string
		testFunc func(logr.Logger, []string) (string, logr.Logger, bool)
	}{
		{"random", sm.randomNextBackend},
		{"roundRobin", func(log logr.Logger, backends []string) (string, logr.Logger, bool) {
			return sm.roundRobinNextBackend(log, "test.host", backends)
		}},
		{"leastConnections", sm.leastConnectionsNextBackend},
		{"lowestLatency", sm.lowestLatencyNextBackend},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, _, ok := tt.testFunc(log, singleBackend)
			require.True(t, ok, "Strategy should return the only backend")
			assert.Equal(t, "only:25565", backend, "Should return the single backend")
		})
	}
}

func TestGetNextBackendStrategyRouting(t *testing.T) {
	sm := NewStrategyManager()
	log := testr.New(t)
	backends := []string{"server1:25565", "server2:25565"}

	tests := []struct {
		strategy config.Strategy
		name     string
	}{
		{config.StrategyRandom, "random"},
		{config.StrategyRoundRobin, "round-robin"},
		{config.StrategyLeastConnections, "least-connections"},
		{config.StrategyLowestLatency, "lowest-latency"},
		{"", "default (empty)"}, // Should default to random
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := &config.Route{
				Strategy: tt.strategy,
			}
			
			backend, _, ok := sm.GetNextBackend(log, route, "test.host", backends)
			require.True(t, ok, "Should return a backend for %s strategy", tt.strategy)
			assert.Contains(t, backends, backend, "Should return one of the configured backends")
		})
	}
}

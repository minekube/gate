package lite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiteAbstraction(t *testing.T) {
	// Create a new Lite instance
	lite := NewLite()
	require.NotNil(t, lite, "NewLite should return a valid instance")

	// Verify it contains a strategy manager
	strategyManager := lite.StrategyManager()
	require.NotNil(t, strategyManager, "Lite should contain a strategy manager")

	// Verify multiple calls return the same instance
	strategyManager2 := lite.StrategyManager()
	assert.Same(t, strategyManager, strategyManager2, "StrategyManager should return the same instance")
}

func TestLiteIsolation(t *testing.T) {
	// Create two separate Lite instances
	lite1 := NewLite()
	lite2 := NewLite()

	// They should be different instances
	assert.NotSame(t, lite1, lite2, "Each NewLite call should return a separate instance")

	// Their strategy managers should also be different
	sm1 := lite1.StrategyManager()
	sm2 := lite2.StrategyManager()
	assert.NotSame(t, sm1, sm2, "Each Lite instance should have its own StrategyManager")

	// Verify state isolation by testing connection counters
	backend := "test:25565"

	// Increment counter in first instance
	decrement1 := sm1.IncrementConnection(backend)
	counter1 := sm1.GetOrCreateCounter(backend)
	assert.Equal(t, uint32(1), counter1.Load(), "Counter in first instance should be 1")

	// Second instance should have separate counter (starts at 0)
	counter2 := sm2.GetOrCreateCounter(backend)
	assert.Equal(t, uint32(0), counter2.Load(), "Counter in second instance should start at 0")

	// Clean up
	decrement1()
	assert.Equal(t, uint32(0), counter1.Load(), "Counter should be decremented")
}

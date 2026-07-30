package lite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrackConnectionCanonicalizesRouteBackendAndReleasesState(t *testing.T) {
	sm := NewStrategyManager()
	release1 := sm.TrackConnection("Route.example.test", "backend.example.test")
	release2 := sm.TrackConnection("route.example.test", "backend.example.test:25565")
	release3 := sm.TrackConnection("other.example.test", "backend.example.test:25565")

	require.Equal(t, uint32(3), sm.ActiveConnections())
	require.Len(t, sm.activeConnections, 2)

	release1()
	release2()
	require.Equal(t, uint32(1), sm.ActiveConnections())
	require.Len(t, sm.activeConnections, 1)

	release3()
	require.Zero(t, sm.ActiveConnections())
	require.Empty(t, sm.activeConnections)
	var strategyCounterCount int
	sm.connectionCounters.Range(func(any, any) bool {
		strategyCounterCount++
		return true
	})
	require.Zero(t, strategyCounterCount)
}

package lite

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
)

// StrategyManager manages state for load balancing strategies across a single Gate instance.
// This eliminates global state and allows multiple Gate instances in the same process.
type StrategyManager struct {
	// Shared random source for all random operations
	rng *rand.Rand

	// Round-robin state per route host
	roundRobinIndexes *sync.Map // map[string]int

	// Connection counters for least-connections strategy
	connectionCounters *sync.Map // map[string]*atomic.Uint32

	// Latency cache for lowest-latency strategy
	latencyCache *ttlcache.Cache[string, time.Duration]
}

// NewStrategyManager creates a new strategy manager for a Gate instance.
func NewStrategyManager() *StrategyManager {
	return &StrategyManager{
		rng:                rand.New(rand.NewSource(time.Now().UnixNano())),
		roundRobinIndexes:  &sync.Map{},
		connectionCounters: &sync.Map{},
		latencyCache:       ttlcache.New[string, time.Duration](),
	}
}

// GetNextBackend returns the next backend using the specified strategy.
func (sm *StrategyManager) GetNextBackend(log logr.Logger, route *config.Route, routeHost string, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	switch route.Strategy {
	case config.StrategyRandom:
		return sm.randomNextBackend(log, backends)
	case config.StrategyRoundRobin:
		return sm.roundRobinNextBackend(log, routeHost, backends)
	case config.StrategyLeastConnections:
		return sm.leastConnectionsNextBackend(log, backends)
	case config.StrategyLowestLatency:
		return sm.lowestLatencyNextBackend(log, backends)
	default:
		// Default to random strategy
		return sm.randomNextBackend(log, backends)
	}
}

// IncrementConnection increments the connection counter for a backend (used with least-connections).
func (sm *StrategyManager) IncrementConnection(backend string) func() {
	if counter := sm.getOrCreateCounter(backend); counter != nil {
		counter.Add(1)
		return func() {
			counter.Add(^uint32(0)) // Decrement on disconnect
		}
	}
	return func() {} // No-op if counter creation failed
}

// RecordLatency records the latency for a backend (used with lowest-latency).
func (sm *StrategyManager) RecordLatency(backend string, latency time.Duration) {
	sm.latencyCache.Set(backend, latency, time.Minute*3)
}

// Private helper methods

func (sm *StrategyManager) randomNextBackend(log logr.Logger, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	// Simple random selection - let tryBackends handle health checking via actual dials
	randIndex := sm.rng.Intn(len(backends))
	backend := backends[randIndex]

	return backend, log, true
}

func (sm *StrategyManager) roundRobinNextBackend(log logr.Logger, routeHost string, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	// Get next backend in round-robin order
	value, _ := sm.roundRobinIndexes.LoadOrStore(routeHost, 0)
	index := value.(int)

	backend := backends[index%len(backends)]
	sm.roundRobinIndexes.Store(routeHost, index+1)

	return backend, log, true
}

func (sm *StrategyManager) leastConnectionsNextBackend(log logr.Logger, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	var leastBackend string
	var leastCount uint32 = math.MaxUint32

	for _, backend := range backends {
		counter := sm.getOrCreateCounter(backend)
		if counter == nil {
			continue
		}

		count := counter.Load()
		if count < leastCount {
			leastBackend = backend
			leastCount = count
		}
	}

	if leastBackend == "" {
		// Fallback to first backend if no counters exist
		return backends[0], log, true
	}

	return leastBackend, log, true
}

func (sm *StrategyManager) lowestLatencyNextBackend(log logr.Logger, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	var lowestBackend string
	var lowestLatency time.Duration

	for _, backend := range backends {
		latencyItem := sm.latencyCache.Get(backend)
		if latencyItem == nil {
			// No latency data yet - this backend will be tried and measured
			// on first successful connection, so prefer it for initial measurement
			return backend, log, true
		}

		if lowestLatency == 0 || latencyItem.Value() < lowestLatency {
			lowestBackend = backend
			lowestLatency = latencyItem.Value()
		}
	}

	if lowestBackend == "" {
		// Fallback to first backend if no latency data exists
		return backends[0], log, true
	}

	return lowestBackend, log, true
}

// GetOrCreateCounter returns the connection counter for a backend (exposed for testing).
func (sm *StrategyManager) GetOrCreateCounter(backend string) *atomic.Uint32 {
	return sm.getOrCreateCounter(backend)
}

func (sm *StrategyManager) getOrCreateCounter(backend string) *atomic.Uint32 {
	value, _ := sm.connectionCounters.LoadOrStore(backend, &atomic.Uint32{})
	counter, ok := value.(*atomic.Uint32)
	if !ok {
		return nil
	}
	return counter
}

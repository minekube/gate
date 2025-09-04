package lite

import (
	"math"
	"math/rand"
	"net"
	"slices"
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
	availableBackends := slices.Clone(backends)
	
	for len(availableBackends) > 0 {
		randIndex := sm.rng.Intn(len(availableBackends))
		backend := availableBackends[randIndex]
		
		if sm.checkBackend(backend) {
			return backend, log, true
		}
		
		availableBackends = slices.Delete(availableBackends, randIndex, randIndex+1)
	}
	
	return "", log, false
}

func (sm *StrategyManager) roundRobinNextBackend(log logr.Logger, routeHost string, backends []string) (string, logr.Logger, bool) {
	for range backends {
		value, _ := sm.roundRobinIndexes.LoadOrStore(routeHost, 0)
		index := value.(int)
		
		backend := backends[index%len(backends)]
		sm.roundRobinIndexes.Store(routeHost, index+1)
		
		if sm.checkBackend(backend) {
			return backend, log, true
		}
	}
	
	return "", log, false
}

func (sm *StrategyManager) leastConnectionsNextBackend(log logr.Logger, backends []string) (string, logr.Logger, bool) {
	var leastBackend string
	var leastCount uint32 = math.MaxUint32
	
	for _, backend := range backends {
		if !sm.checkBackend(backend) {
			continue
		}
		
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
		return "", log, false
	}
	
	return leastBackend, log, true
}

func (sm *StrategyManager) lowestLatencyNextBackend(log logr.Logger, backends []string) (string, logr.Logger, bool) {
	var lowestBackend string
	var lowestLatency time.Duration
	
	for _, backend := range backends {
		if !sm.checkBackend(backend) {
			continue
		}
		
		latencyItem := sm.latencyCache.Get(backend)
		if latencyItem == nil {
			// Set a default latency that will be updated on first actual dial
			sm.latencyCache.Set(backend, time.Nanosecond, time.Minute*3)
			latencyItem = sm.latencyCache.Get(backend)
		}
		
		if latencyItem != nil && (lowestLatency == 0 || latencyItem.Value() < lowestLatency) {
			lowestBackend = backend
			lowestLatency = latencyItem.Value()
		}
	}
	
	if lowestBackend == "" {
		return "", log, false
	}
	
	return lowestBackend, log, true
}

func (sm *StrategyManager) getOrCreateCounter(backend string) *atomic.Uint32 {
	value, _ := sm.connectionCounters.LoadOrStore(backend, &atomic.Uint32{})
	counter, ok := value.(*atomic.Uint32)
	if !ok {
		return nil
	}
	return counter
}

func (sm *StrategyManager) checkBackend(backend string) bool {
	conn, err := net.DialTimeout("tcp", backend, time.Second*5)
	if err == nil {
		_ = conn.Close()
	}
	return err == nil
}

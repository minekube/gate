package lite

import (
	"math"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/util/netutil"
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
	strategyCountersMu sync.Mutex

	activeConnectionsMu sync.RWMutex
	activeConnections   map[string]uint32

	// Latency cache for lowest-latency strategy
	latencyCache *ttlcache.Cache[string, time.Duration]
}

// NewStrategyManager creates a new strategy manager for a Gate instance.
func NewStrategyManager() *StrategyManager {
	return &StrategyManager{
		rng:                rand.New(rand.NewSource(time.Now().UnixNano())),
		roundRobinIndexes:  &sync.Map{},
		connectionCounters: &sync.Map{},
		activeConnections:  make(map[string]uint32),
		latencyCache:       ttlcache.New[string, time.Duration](),
	}
}

// GetNextBackend returns the next backend using the specified strategy.
func (sm *StrategyManager) GetNextBackend(log logr.Logger, route *config.Route, routeHost string, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	switch route.Strategy {
	case config.StrategySequential:
		return sm.sequentialNextBackend(log, backends)
	case config.StrategyRandom:
		return sm.randomNextBackend(log, backends)
	case config.StrategyRoundRobin:
		return sm.roundRobinNextBackend(log, routeHost, backends)
	case config.StrategyLeastConnections:
		return sm.leastConnectionsNextBackend(log, backends)
	case config.StrategyLowestLatency:
		return sm.lowestLatencyNextBackend(log, backends)
	case "":
		// Default to sequential strategy when no strategy is defined
		return sm.sequentialNextBackend(log, backends)
	default:
		// Default to sequential strategy for unknown strategies
		return sm.sequentialNextBackend(log, backends)
	}
}

// IncrementConnection increments the connection counter for a backend (used with least-connections).
func (sm *StrategyManager) IncrementConnection(backend string) func() {
	sm.strategyCountersMu.Lock()
	counter := sm.getOrCreateCounter(backend)
	if counter == nil {
		sm.strategyCountersMu.Unlock()
		return func() {}
	}
	counter.Add(1)
	sm.strategyCountersMu.Unlock()

	return func() {
		sm.strategyCountersMu.Lock()
		defer sm.strategyCountersMu.Unlock()
		if counter.Load() == 0 {
			return
		}
		counter.Add(^uint32(0))
		if counter.Load() == 0 {
			sm.connectionCounters.CompareAndDelete(backend, counter)
		}
	}
}

func (sm *StrategyManager) TrackConnection(routeHost, backend string) func() {
	key := canonicalConnectionKey(routeHost, backend)
	sm.activeConnectionsMu.Lock()
	sm.activeConnections[key]++
	sm.activeConnectionsMu.Unlock()

	decrementStrategyCounter := sm.IncrementConnection(backend)
	return func() {
		decrementStrategyCounter()

		sm.activeConnectionsMu.Lock()
		if count := sm.activeConnections[key]; count <= 1 {
			delete(sm.activeConnections, key)
		} else {
			sm.activeConnections[key] = count - 1
		}
		sm.activeConnectionsMu.Unlock()
	}
}

func (sm *StrategyManager) ActiveConnections() uint32 {
	sm.activeConnectionsMu.RLock()
	defer sm.activeConnectionsMu.RUnlock()

	var total uint32
	for _, count := range sm.activeConnections {
		total += count
	}
	return total
}

func canonicalConnectionKey(routeHost, backend string) string {
	return strings.ToLower(routeHost) + "\x00" + canonicalBackendAddress(backend)
}

func canonicalBackendAddress(backend string) string {
	addr, err := netutil.Parse(backend, "tcp")
	if err != nil {
		return strings.ToLower(backend)
	}
	host, port := netutil.HostPort(addr)
	if port == 0 {
		port = 25565
	}
	return net.JoinHostPort(strings.ToLower(host), strconv.Itoa(int(port)))
}

// RecordLatency records the latency for a backend (used with lowest-latency).
func (sm *StrategyManager) RecordLatency(backend string, latency time.Duration) {
	sm.latencyCache.Set(backend, latency, time.Minute*3)
}

// Private helper methods

func (sm *StrategyManager) sequentialNextBackend(log logr.Logger, backends []string) (string, logr.Logger, bool) {
	if len(backends) == 0 {
		return "", log, false
	}

	// Sequential strategy always returns the first backend in the list
	// The caller (tryBackends) will remove failed backends from the list
	backend := backends[0]
	return backend, log, true
}

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
		var count uint32
		if counter := sm.getCounter(backend); counter != nil {
			count = counter.Load()
		}
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

func (sm *StrategyManager) getCounter(backend string) *atomic.Uint32 {
	value, ok := sm.connectionCounters.Load(backend)
	if !ok {
		return nil
	}
	counter, ok := value.(*atomic.Uint32)
	if !ok {
		return nil
	}
	return counter
}

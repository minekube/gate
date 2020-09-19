package addrquota

import (
	"github.com/golang/groupcache/lru"
	"golang.org/x/time/rate"
	"net"
	"sync"
)

// Quota implements a simple IP-based rate limiter.
// Each set of incoming IP addresses with the same
// low-order byte gets events per second.
// Information is kept in an LRU cache of size maxEntries.
type Quota struct {
	eps   float32    // allowed events per second
	burst int        // maximum events per second (queue)
	mu    sync.Mutex // protects cache
	cache *lru.Cache
}

func (q *Quota) Blocked(addr net.Addr) bool {
	var limiter *rate.Limiter
	key := ipKey(addr)
	if key != "" {
		q.mu.Lock()
		if v, ok := q.cache.Get(key); ok {
			limiter = v.(*rate.Limiter)
		} else {
			limiter = rate.NewLimiter(rate.Limit(q.eps), q.burst)
			q.cache.Add(key, limiter)
		}
		q.mu.Unlock()
	}
	return limiter != nil && !limiter.Allow()
}

func NewQuota(eventsPerSecond float32, burst, maxEntries int) *Quota {
	return &Quota{
		eps:   eventsPerSecond,
		burst: burst,
		cache: lru.New(maxEntries),
	}
}

func ipKey(addr net.Addr) string {
	host, _, _ := net.SplitHostPort(addr.String())
	ip := net.ParseIP(host)
	if ip == nil {
		return ""
	}
	// Zero out last byte, to cover ranges.
	ip[len(ip)-1] = 0
	return ip.String()
}

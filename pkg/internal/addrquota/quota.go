package addrquota

import (
	"github.com/golang/groupcache/lru"
	"golang.org/x/time/rate"
	"net"
	"sync"
)

// Quota implements a simple IP-based rate limiter.
// Incoming addresses are bucketed by network prefix (IPv4 /24, IPv6 /64,
// see ipKey) and each bucket gets the configured events per second.
// Information is kept in an LRU cache of size maxEntries.
type Quota struct {
	eps   float32    // allowed events per second
	burst int        // maximum events per second (queue)
	mu    sync.Mutex // protects cache
	cache *lru.Cache
}

func (q *Quota) Blocked(ip string) bool {
	var limiter *rate.Limiter
	key := ipKey(ip)
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

// ipKey derives the quota bucket key for an IP address.
//
// IPv4 addresses (including IPv4-mapped IPv6 addresses) are bucketed by their
// /24 prefix. IPv6 addresses are bucketed by their /64 prefix: a /64 is the
// standard allocation handed to a single end user, so bucketing any narrower
// would let one subscriber rotate through its own addresses and evade the
// limiter entirely.
func ipKey(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String()
}

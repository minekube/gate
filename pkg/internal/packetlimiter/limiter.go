// Package packetlimiter provides a per-connection serverbound packet rate
// limiter. It bounds how many packets and/or bytes a single connection may send
// to the proxy over a sliding time window, mitigating packet-flood abuse from
// already-connected clients (Gate's other quotas only apply at connect/login).
package packetlimiter

import (
	"sync"
	"time"
)

// Limiter enforces a per-connection limit on packets and/or bytes over a
// sliding window. A nil *Limiter allows everything (disabled). Safe for
// concurrent use.
type Limiter struct {
	mu               sync.Mutex
	packets          *counter // nil if packet limiting is disabled
	bytes            *counter // nil if byte limiting is disabled
	packetsPerSecond int
	bytesPerSecond   int
}

// New returns a Limiter for the given per-second limits over window, or nil if
// limiting is effectively disabled (both limits <= 0, or window <= 0). A limit
// <= 0 disables that dimension while the other stays active.
func New(packetsPerSecond, bytesPerSecond int, window time.Duration) *Limiter {
	if window <= 0 || (packetsPerSecond <= 0 && bytesPerSecond <= 0) {
		return nil
	}
	l := &Limiter{
		packetsPerSecond: packetsPerSecond,
		bytesPerSecond:   bytesPerSecond,
	}
	if packetsPerSecond > 0 {
		l.packets = newCounter(window)
	}
	if bytesPerSecond > 0 {
		l.bytes = newCounter(window)
	}
	return l
}

// Account records one packet of the given size and reports whether the
// connection is still within its limits. It returns false once a configured
// rate is exceeded, signalling the caller to drop the connection.
func (l *Limiter) Account(bytes int) bool {
	if l == nil {
		return true
	}
	now := time.Now().UnixNano()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.packets != nil {
		l.packets.updateAndAdd(1, now)
		if l.packets.rate() > float64(l.packetsPerSecond) {
			return false
		}
	}
	if l.bytes != nil {
		l.bytes.updateAndAdd(int64(bytes), now)
		if l.bytes.rate() > float64(l.bytesPerSecond) {
			return false
		}
	}
	return true
}

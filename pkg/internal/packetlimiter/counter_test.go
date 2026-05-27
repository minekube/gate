package packetlimiter

import (
	"testing"
	"time"
)

func TestCounterAccumulatesWithinWindow(t *testing.T) {
	c := newCounter(time.Second)
	base := int64(1_000_000_000)
	c.updateAndAdd(10, base)
	if got := c.sum(); got != 10 {
		t.Fatalf("sum after first add = %d, want 10", got)
	}
	c.updateAndAdd(5, base+int64(500*time.Millisecond))
	if got := c.sum(); got != 15 {
		t.Fatalf("sum within window = %d, want 15", got)
	}
}

func TestCounterExpiresOldEntries(t *testing.T) {
	c := newCounter(time.Second)
	base := int64(1_000_000_000)
	c.updateAndAdd(10, base)                             // expires at base+1s
	c.updateAndAdd(5, base+int64(500*time.Millisecond))  // still in window at base+1.5s
	c.updateAndAdd(1, base+int64(1500*time.Millisecond)) // drops the base(10) entry
	if got := c.sum(); got != 6 {                        // 5 + 1
		t.Fatalf("sum after expiry = %d, want 6", got)
	}
}

func TestCounterRatePerSecond(t *testing.T) {
	c := newCounter(7 * time.Second)
	now := int64(10_000_000_000)
	for i := 0; i < 700; i++ {
		c.updateAndAdd(1, now)
	}
	// rate = sum / windowSeconds = 700 / 7 = 100 per second
	if got := c.rate(); got != 100 {
		t.Fatalf("rate = %v, want 100", got)
	}
}

// Many entries beyond the initial backing size must not corrupt the ring buffer.
func TestCounterResizeKeepsSum(t *testing.T) {
	c := newCounter(time.Hour) // huge window so nothing expires
	now := int64(1_000_000_000)
	for i := 0; i < 1000; i++ {
		c.updateAndAdd(2, now+int64(i)) // distinct timestamps
	}
	if got := c.sum(); got != 2000 {
		t.Fatalf("sum after 1000 adds = %d, want 2000", got)
	}
}

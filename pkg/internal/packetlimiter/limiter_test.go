package packetlimiter

import (
	"testing"
	"time"
)

func TestLimiterAllowsUnderPacketRate(t *testing.T) {
	l := New(500, -1, 7*time.Second)
	// 100 packets in a burst: rate = 100/7 ≈ 14/s, well under 500/s.
	for i := 0; i < 100; i++ {
		if !l.Account(10) {
			t.Fatalf("packet %d rejected while under the limit", i)
		}
	}
}

func TestLimiterRejectsOverPacketRate(t *testing.T) {
	l := New(500, -1, 7*time.Second)
	// rate exceeds 500/s once sum/7 > 500, i.e. > 3500 packets in the window.
	rejected := false
	for i := 0; i < 6000; i++ {
		if !l.Account(10) {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Fatal("expected rejection once the packet rate was exceeded")
	}
}

func TestLimiterRejectsOverByteRate(t *testing.T) {
	l := New(0, 1000, 7*time.Second) // packets disabled, 1000 bytes/s
	rejected := false
	for i := 0; i < 1000; i++ {
		if !l.Account(100) { // bytes accumulate; rate > 1000/s once sum > 7000
			rejected = true
			break
		}
	}
	if !rejected {
		t.Fatal("expected rejection once the byte rate was exceeded")
	}
}

func TestNewDisabledReturnsNilAndAccountsTrue(t *testing.T) {
	if l := New(0, 0, 7*time.Second); l != nil {
		t.Fatal("expected nil limiter when both rates are disabled")
	}
	if l := New(500, -1, 0); l != nil {
		t.Fatal("expected nil limiter when window is non-positive")
	}
	var nilLimiter *Limiter
	if !nilLimiter.Account(123456) {
		t.Fatal("nil limiter must allow everything")
	}
}

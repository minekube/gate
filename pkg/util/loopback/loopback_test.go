package loopback

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
)

func mustAllocator(t *testing.T, rng Range) *Allocator {
	t.Helper()
	a, err := NewAllocator(rng, "127.0.0.1")
	if err != nil {
		t.Fatalf("NewAllocator(%s): %v", rng, err)
	}
	return a
}

// TestAllocatorNeverHandsOutAnAddressTwice is the regression guard for the
// 2026-10-20 silent stall (kanban t_40da126d): a dynamic backend's address is
// reserved with listen-then-close, and a second consumer that is handed the same
// port kills the first one's runtime ("Address already in use") while readiness
// still passes. 1000 concurrent allocations must all be distinct.
func TestAllocatorNeverHandsOutAnAddressTwice(t *testing.T) {
	a := mustAllocator(t, Range{Start: 30000, End: 31999})
	const allocations = 1000

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		addrs = make(map[string]int, allocations)
		errs  []error
	)
	listeners := make([]net.Listener, 0, allocations)
	var listenersMu sync.Mutex
	for i := 0; i < allocations; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ln, err := a.Listen()
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			addr := ln.Addr().String()
			mu.Lock()
			if prev, dup := addrs[addr]; dup {
				errs = append(errs, fmt.Errorf("allocations %d and %d were both handed %s", prev, i, addr))
			}
			addrs[addr] = i
			mu.Unlock()
			listenersMu.Lock()
			listeners = append(listeners, ln)
			listenersMu.Unlock()
		}(i)
	}
	wg.Wait()
	for _, ln := range listeners {
		_ = ln.Close()
	}
	for _, err := range errs {
		t.Error(err)
	}
	if len(addrs) != allocations {
		t.Fatalf("%d distinct addresses for %d allocations", len(addrs), allocations)
	}
	ephemeral := EphemeralRange()
	for addr := range addrs {
		port, ok := PortOf(addr)
		if !ok {
			t.Fatalf("unparseable allocated address %q", addr)
		}
		if ephemeral.ContainsPort(port) {
			t.Errorf("allocated %s from the kernel ephemeral range %s", addr, ephemeral)
		}
		if !a.Range().ContainsPort(port) {
			t.Errorf("allocated %s outside the reserved range %s", addr, a.Range())
		}
	}
}

// TestReservedAddressIsNeverHandedToAListener pins the exact collision shape: a
// consumer reserved an address with listen-then-close and a listener allocated
// afterwards must not be handed that port.
func TestReservedAddressIsNeverHandedToAListener(t *testing.T) {
	a := mustAllocator(t, Range{Start: 30000, End: 31999})

	reserved, release, err := a.Reserve()
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !a.HandedOut(reserved) {
		t.Fatalf("HandedOut(%s) = false, want true while the reservation is held", reserved)
	}

	for i := 0; i < 200; i++ {
		ln, err := a.Listen()
		if err != nil {
			t.Fatalf("Listen #%d: %v", i, err)
		}
		got := ln.Addr().String()
		_ = ln.Close()
		if got == reserved {
			t.Fatalf("listener #%d was handed the reserved address %s: its bind would take the port "+
				"the runtime reserved, which kills the runtime and routes joins into a foreign tunnel", i, got)
		}
	}

	release()
	release() // releasing twice must be safe
	if a.HandedOut(reserved) {
		t.Fatalf("HandedOut(%s) = true after release", reserved)
	}
}

// TestAllocatorFailsClosedWhenTheRangeIsFull pins the rule that an exhausted
// range is an error, never a fallback to a kernel-chosen ephemeral port: the
// fallback re-opens the collision window the allocator exists to close.
func TestAllocatorFailsClosedWhenTheRangeIsFull(t *testing.T) {
	a := mustAllocator(t, Range{Start: 30000, End: 30001})

	first, err := a.Listen()
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	defer first.Close()
	second, err := a.Listen()
	if err != nil {
		t.Fatalf("second Listen: %v", err)
	}
	defer second.Close()

	third, err := a.Listen()
	if err == nil {
		_ = third.Close()
		t.Fatal("third Listen on a full range returned a port, want an exhaustion error: a " +
			"kernel-chosen fallback port can be taken by another consumer of the same port")
	}
	if !errors.Is(err, ErrExhausted) {
		t.Fatalf("third Listen error = %v, want ErrExhausted", err)
	}
	if _, _, err := a.Reserve(); !errors.Is(err, ErrExhausted) {
		t.Fatalf("Reserve on a full range error = %v, want ErrExhausted", err)
	}
}

// TestNewAllocatorRefusesRangesThatCanCollide pins every fail-closed rule the
// allocator relies on to keep its listeners out of another allocator's way.
func TestNewAllocatorRefusesRangesThatCanCollide(t *testing.T) {
	ephemeral := EphemeralRange()
	tests := []struct {
		name string
		rng  Range
		host string
		want string
	}{
		{
			name: "ephemeral range",
			rng:  Range{Start: ephemeral.Start, End: ephemeral.Start + 9},
			host: "127.0.0.1",
			want: "ephemeral",
		},
		{
			name: "vialite dynamic backend range",
			rng:  VialiteDynamicRange(),
			host: "127.0.0.1",
			want: "vialite",
		},
		{
			name: "overlapping vialite range",
			rng:  Range{Start: VialiteDynamicRangeEnd - 1, End: VialiteDynamicRangeEnd + 1},
			host: "127.0.0.1",
			want: "vialite",
		},
		{
			name: "non-loopback host",
			rng:  Range{Start: 30000, End: 31999},
			host: "0.0.0.0",
			want: "loopback",
		},
		{
			name: "privileged range",
			rng:  Range{Start: 80, End: 90},
			host: "127.0.0.1",
			want: "1024",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewAllocator(tt.rng, tt.host)
			if err == nil {
				t.Fatalf("NewAllocator(%s, %q) = %v, want an error", tt.rng, tt.host, a)
			}
			if !errors.Is(err, ErrRangeUnusable) {
				t.Fatalf("NewAllocator error = %v, want ErrRangeUnusable", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

// TestBridgeRangeIsDisjoint keeps the shipped default range and the range the
// vialite runtime reserves apart, and both outside the kernel's ephemeral range.
func TestBridgeRangeIsDisjoint(t *testing.T) {
	bridge := Range{Start: BridgeRangeStart, End: BridgeRangeEnd}
	vialiteRange := VialiteDynamicRange()
	ephemeral := EphemeralRange()
	if bridge.Overlaps(vialiteRange) {
		t.Fatalf("Gate's bridge range %s overlaps the vialite dynamic backend range %s", bridge, vialiteRange)
	}
	if bridge.Overlaps(ephemeral) {
		t.Fatalf("Gate's bridge range %s overlaps the kernel ephemeral range %s", bridge, ephemeral)
	}
	if _, err := NewAllocator(bridge, "127.0.0.1"); err != nil {
		t.Fatalf("the shipped bridge range is not usable on this host: %v", err)
	}
}

// TestBridgeRangeEnvOverride pins the operator escape hatch for hosts whose
// ephemeral range makes the default unusable.
func TestBridgeRangeEnvOverride(t *testing.T) {
	resetDefault()
	t.Cleanup(resetDefault)
	t.Setenv(EnvRange, "30500-30510")

	a, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if got, want := a.Range(), (Range{Start: 30500, End: 30510}); got != want {
		t.Fatalf("Default().Range() = %s, want %s", got, want)
	}
	ln, err := a.Listen()
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	port, _ := PortOf(ln.Addr().String())
	if port < 30500 || port > 30510 {
		t.Fatalf("listener %s is outside the configured range %s", ln.Addr(), a.Range())
	}
}

//go:build !musl

package proxy

import (
	"context"
	"net"
	"testing"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/util/loopback"
	vialite "go.minekube.com/vialite"
)

// The template for the tests in this file: one loopback allocator per process.
//
// Gate owns the loopback ports of every dynamic listener it creates (the
// per-join translation bridges) and now also hands the vialite runtime the
// address each dynamic backend is told to bind, through
// vialite.Options.ReserveLoopbackAddress. Neither consumer may ever name a port
// the other is holding, and a dynamically allocated address must never come from
// the kernel's ephemeral range (kanban t_abef60dc / t_6199659e).

// TestViaManagedRunnerInjectsGateLoopbackAllocator pins the wiring: the runtime
// is constructed with Gate's allocator, so dynamic backend addresses come out of
// the same reserved range as Gate's own listeners.
func TestViaManagedRunnerInjectsGateLoopbackAllocator(t *testing.T) {
	runner := newViaManagedRunner(&config.Config{Via: config.Via{Enabled: true}})

	opts, err := runner.options()
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.ReserveLoopbackAddress == nil {
		t.Fatal("options().ReserveLoopbackAddress = nil: a dynamic backend's address is then " +
			"allocated by the runtime from a second range, so Gate cannot guarantee that no two " +
			"dynamic loopback listeners in this process share a port")
	}

	alloc, err := loopback.Default()
	if err != nil {
		t.Fatalf("loopback.Default: %v", err)
	}

	first, releaseFirst, err := opts.ReserveLoopbackAddress("127.0.0.1")
	if err != nil {
		t.Fatalf("ReserveLoopbackAddress: %v", err)
	}
	if releaseFirst == nil {
		t.Fatal("ReserveLoopbackAddress returned a nil release: the address would stay reserved for the process lifetime")
	}
	host, _, err := net.SplitHostPort(first)
	if err != nil {
		t.Fatalf("ReserveLoopbackAddress = %q: %v", first, err)
	}
	if host != alloc.Host() {
		t.Fatalf("reserved address host = %q, want the allocator host %q", host, alloc.Host())
	}
	if !alloc.Range().ContainsAddress(first) {
		t.Fatalf("reserved address %s is outside Gate's reserved range %s: a dynamic backend there "+
			"can be handed a port in the kernel's ephemeral range, where a listen-then-close "+
			"reservation is stealable", first, alloc.Range())
	}
	if !alloc.HandedOut(first) {
		t.Fatalf("allocator does not hold %s after reserving it for the runtime", first)
	}

	second, releaseSecond, err := opts.ReserveLoopbackAddress("127.0.0.1")
	if err != nil {
		t.Fatalf("second ReserveLoopbackAddress: %v", err)
	}
	if second == first {
		t.Fatalf("two dynamic backends were handed the same address %s", first)
	}
	releaseSecond()

	// Release is exactly-once and returns the port: calling it twice must not
	// panic and must leave the allocator free of the address.
	releaseFirst()
	releaseFirst()
	if alloc.HandedOut(first) {
		t.Fatalf("allocator still holds %s after the runtime's release ran", first)
	}
}

// TestViaManagedRunnerStartPassesAllocatorToRuntime proves the option reaches the
// runtime constructor, not just options().
func TestViaManagedRunnerStartPassesAllocatorToRuntime(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{"lobby": "127.0.0.1:25566"},
		Via:     config.Via{Enabled: true},
	}
	runner := newViaManagedRunner(cfg)
	fake := &fakeVialiteServer{backends: map[string]string{"lobby": "127.0.0.1:25590"}}
	var got vialite.Options
	runner.newServer = func(opts vialite.Options) (vialiteServer, error) {
		got = opts
		return fake, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer runner.Stop()

	if got.ReserveLoopbackAddress == nil {
		t.Fatal("the runtime was constructed without ReserveLoopbackAddress")
	}
}

// TestViaBackendDialAddressAcceptsAddressTheAllocatorReserved is the regression
// guard for the adoption itself: a dynamic backend's published address now comes
// from Gate's own bridge range, and the ownership check must accept exactly the
// addresses Gate handed to the runtime - it may not refuse them as "Gate's own
// bridge range".
func TestViaBackendDialAddressAcceptsAddressTheAllocatorReserved(t *testing.T) {
	runner := newViaManagedRunner(&config.Config{Via: config.Via{Enabled: true}})
	opts, err := runner.options()
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.ReserveLoopbackAddress == nil {
		t.Fatal("options().ReserveLoopbackAddress = nil: the runtime allocates dynamic backend " +
			"addresses from a second range, so Gate cannot keep them apart from its own listeners")
	}
	addr, release, err := opts.ReserveLoopbackAddress("")
	if err != nil {
		t.Fatalf("ReserveLoopbackAddress: %v", err)
	}
	defer release()

	runner.server = &fakeVialiteServer{backends: map[string]string{"dyn-lobby": addr}}
	runner.dynamicBackends = map[string]*viaDynamicBackend{"dyn-lobby": {}}

	got, err := runner.BackendDialAddress("dyn-lobby")
	if err != nil {
		t.Fatalf("BackendDialAddress(dyn-lobby) = %v, want the address Gate's allocator reserved for "+
			"that runtime: refusing it fails every translated join once the allocator is injected", err)
	}
	if got != addr {
		t.Fatalf("BackendDialAddress = %q, want %q", got, addr)
	}

	// The acceptance is bound to the live reservation: once the runtime released
	// the address, publishing it again is the stale-publish case the range check
	// exists for.
	release()
	if got, err := runner.BackendDialAddress("dyn-lobby"); err == nil {
		t.Fatalf("BackendDialAddress(dyn-lobby) = %q, nil after the reservation was released: a "+
			"released address can be handed to a bridge at any moment", got)
	}
}

// TestViaBackendDialAddressRefusesUnreservedAddressInBridgeRange is the control
// for the test above: an address inside Gate's range that Gate never handed to
// the runtime (for example a bridge listener of this process) stays refused.
func TestViaBackendDialAddressRefusesUnreservedAddressInBridgeRange(t *testing.T) {
	runner := newViaManagedRunner(&config.Config{Via: config.Via{Enabled: true}})
	if _, err := runner.options(); err != nil {
		t.Fatalf("options: %v", err)
	}
	bridge := mustBridge(t, "dyn-other")
	runner.server = &fakeVialiteServer{backends: map[string]string{"dyn-lobby": bridge.Addr().String()}}
	runner.dynamicBackends = map[string]*viaDynamicBackend{"dyn-lobby": {}}

	if got, err := runner.BackendDialAddress("dyn-lobby"); err == nil {
		t.Fatalf("BackendDialAddress(dyn-lobby) = %q, nil: %s is a live listener of this Gate "+
			"process that Gate never reserved for the runtime", got, bridge.Addr())
	}
}

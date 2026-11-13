package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The tests in this file pin the loopback-port ownership rules for Gate's
// per-join translation bridge. They are the regression guard for the silent
// config-state stall measured on 2026-10-20 (kanban t_40da126d): the vialite
// supervisor reserved a dynamic backend's loopback port with a listen-then-close
// and Gate's bridge listener - allocated the same way, from the kernel's
// ephemeral range - took that port in an 8-way join burst. The child then died
// with "Address already in use", readiness ("something answers a dial") was
// satisfied by Gate's own bridge, the dead backend was published anyway, and one
// player's join was routed into another session's tunnel while both stalled in
// configuration state.

// unusedServerDialer satisfies ServerDialer for bridges that are only inspected,
// never dialled.
type unusedServerDialer struct{}

func (unusedServerDialer) Dial(context.Context, Player) (net.Conn, error) {
	return nil, errors.New("unused")
}

// hostEphemeralRange returns the kernel's ephemeral (anonymous) port range.
func hostEphemeralRange(t *testing.T) (int, int) {
	t.Helper()
	raw, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err != nil {
		// Linux default; the ephemeral range is what makes a listen-then-close
		// reservation stealable, so assuming the default is the safe direction.
		return 32768, 60999
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		t.Fatalf("unparseable ip_local_port_range %q", string(raw))
	}
	start, err := strconv.Atoi(fields[0])
	if err != nil {
		t.Fatalf("unparseable ip_local_port_range start %q: %v", fields[0], err)
	}
	end, err := strconv.Atoi(fields[1])
	if err != nil {
		t.Fatalf("unparseable ip_local_port_range end %q: %v", fields[1], err)
	}
	return start, end
}

func portOfAddr(t *testing.T, addr net.Addr) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port %q: %v", portStr, err)
	}
	return port
}

// dynamicRunner returns a runner that has registered name as a dynamic backend,
// optionally with a translation bridge.
func dynamicRunner(name string, bridge *viaBackendBridge, published string) *viaManagedRunner {
	runner := &viaManagedRunner{
		server:          &fakeVialiteServer{backends: map[string]string{name: published}},
		dynamicBackends: map[string]*viaDynamicBackend{name: {bridge: bridge}},
	}
	return runner
}

func mustBridge(t *testing.T, name string) *viaBackendBridge {
	t.Helper()
	bridge, err := newViaBackendBridge(unusedServerDialer{}, name, 0)
	if err != nil {
		t.Fatalf("newViaBackendBridge(%s): %v", name, err)
	}
	t.Cleanup(func() { _ = bridge.Close() })
	return bridge
}

// TestViaBridgeListenerAvoidsTheEphemeralPortRange is the regression guard for
// the allocation half of the 2026-10-20 stall.
//
// A bridge listener is kept for the whole join, so its port must not come out of
// the kernel's ephemeral range: that range is exactly where every
// listen-then-close reservation in the process (the vialite supervisor's dynamic
// backend address) is drawn from, and a kept listener that takes such a
// reservation's port kills the child that is about to bind it.
func TestViaBridgeListenerAvoidsTheEphemeralPortRange(t *testing.T) {
	bridge := mustBridge(t, "dyn-lobby")

	port := portOfAddr(t, bridge.Addr())
	start, end := hostEphemeralRange(t)
	if port >= start && port <= end {
		t.Fatalf("bridge listener %s was allocated from the kernel ephemeral range %d-%d: a listener "+
			"there can take the port a dynamic backend reserved moments earlier, which kills that "+
			"runtime with \"Address already in use\" and routes its joins into another session's tunnel",
			bridge.Addr(), start, end)
	}
	if port < 30000 || port > 31999 {
		t.Fatalf("bridge listener %s is outside Gate's reserved bridge port range 30000-31999; every "+
			"dynamically allocated loopback listener must come from one dedicated range so it can "+
			"never collide with a runtime's reservation", bridge.Addr())
	}
}

// TestViaBackendDialAddressAcceptsTheBackendsOwnBridge is the control for the
// ownership check: Gate publishes a bridged backend's own bridge address, and
// that address must stay dialable. Without this the fix would refuse every
// translated join.
func TestViaBackendDialAddressAcceptsTheBackendsOwnBridge(t *testing.T) {
	bridge := mustBridge(t, "dyn-lobby")
	runner := dynamicRunner("dyn-lobby", bridge, bridge.Addr().String())

	addr, err := runner.BackendDialAddress("dyn-lobby")
	if err != nil {
		t.Fatalf("BackendDialAddress(dyn-lobby) = %v, want the bridge address", err)
	}
	if addr != bridge.Addr().String() {
		t.Fatalf("BackendDialAddress = %q, want %q", addr, bridge.Addr().String())
	}
}

// TestViaBackendDialAddressRefusesAnotherBackendsBridge is the exact measured
// shape: backend B's published address is the bridge listener of backend A
// because A's bridge took the port B's runtime had reserved. Dialling it would
// serve B's join out of A's tunnel.
func TestViaBackendDialAddressRefusesAnotherBackendsBridge(t *testing.T) {
	bridgeA := mustBridge(t, "dyn-a")
	bridgeB := mustBridge(t, "dyn-b")

	// A's bridge is what answers B's published address.
	runner := &viaManagedRunner{
		server: &fakeVialiteServer{backends: map[string]string{"dyn-b": bridgeA.Addr().String()}},
		dynamicBackends: map[string]*viaDynamicBackend{
			"dyn-a": {bridge: bridgeA},
			"dyn-b": {bridge: bridgeB},
		},
	}

	addr, err := runner.BackendDialAddress("dyn-b")
	if err == nil {
		t.Fatalf("BackendDialAddress(dyn-b) = %q, nil: that address is dyn-a's bridge listener, so "+
			"the join would be routed into another session's tunnel", addr)
	}
	for _, want := range []string{bridgeA.Addr().String(), "dyn-a", "dyn-b", "another session"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestViaBackendDialAddressRefusesAStrayBridgeListener covers the same collision
// before backend bookkeeping knows the bridge: a live listener of this Gate
// process is never a runtime's backend address.
func TestViaBackendDialAddressRefusesAStrayBridgeListener(t *testing.T) {
	bridge := mustBridge(t, "dyn-lobby")
	runner := dynamicRunner("dyn-lobby", nil, bridge.Addr().String())

	addr, err := runner.BackendDialAddress("dyn-lobby")
	if err == nil {
		t.Fatalf("BackendDialAddress(dyn-lobby) = %q, nil: that address is a live bridge listener of "+
			"this Gate process, so the runtime cannot be serving it", addr)
	}
	for _, want := range []string{bridge.Addr().String(), "dyn-lobby", "another session"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestViaBackendDialAddressRefusesAddressInBridgePortRange covers the state the
// collision leaves behind when the bridge listener is gone but the address was
// already published: a backend address inside the range reserved for Gate's own
// listeners can be handed to a bridge at any moment.
func TestViaBackendDialAddressRefusesAddressInBridgePortRange(t *testing.T) {
	runner := dynamicRunner("dyn-lobby", nil, "127.0.0.1:31001")

	addr, err := runner.BackendDialAddress("dyn-lobby")
	if err == nil {
		t.Fatalf("BackendDialAddress(dyn-lobby) = %q, nil: 31001 is inside the range reserved for Gate's "+
			"per-join bridge listeners, so it can be handed to a bridge at any moment", addr)
	}
	for _, want := range []string{"127.0.0.1:31001", "dyn-lobby", "30000-31999"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestViaBackendDialAddressLeavesStaticBackendsAlone keeps the ownership rules
// off configured backends: their addresses are operator configuration, not ports
// Gate allocated, even when they use a port inside the bridge range.
func TestViaBackendDialAddressLeavesStaticBackendsAlone(t *testing.T) {
	runner := &viaManagedRunner{
		server: &fakeVialiteServer{backends: map[string]string{"lobby": "127.0.0.1:31002"}},
	}
	got, err := runner.BackendDialAddress("lobby")
	if err != nil {
		t.Fatalf("BackendDialAddress(lobby) = %v, want the configured address", err)
	}
	if got != "127.0.0.1:31002" {
		t.Fatalf("BackendDialAddress = %q, want 127.0.0.1:31002", got)
	}
}

// TestViaBridgeListenersStayUniqueUnderConcurrentJoins checks the allocator hand
// out under the shape that produced the stall: many bridges live at once.
func TestViaBridgeListenersStayUniqueUnderConcurrentJoins(t *testing.T) {
	const joins = 200
	bridges := make(chan *viaBackendBridge, joins)
	errs := make(chan error, joins)
	for i := 0; i < joins; i++ {
		go func(i int) {
			bridge, err := newViaBackendBridge(unusedServerDialer{}, fmt.Sprintf("dyn-%d", i), 0)
			if err != nil {
				errs <- err
				return
			}
			// Held open until every allocation is collected: a closed bridge
			// releases its port, and reusing a released port is correct.
			bridges <- bridge
		}(i)
	}
	held := make([]*viaBackendBridge, 0, joins)
	defer func() {
		for _, bridge := range held {
			_ = bridge.Close()
		}
	}()
	for i := 0; i < joins; i++ {
		select {
		case err := <-errs:
			t.Fatalf("bridge allocation failed: %v", err)
		case bridge := <-bridges:
			held = append(held, bridge)
		case <-time.After(30 * time.Second):
			t.Fatalf("only %d of %d bridge allocations completed", len(held), joins)
		}
	}

	seen := make(map[string]bool, joins)
	start, end := hostEphemeralRange(t)
	for _, bridge := range held {
		addr := bridge.Addr().String()
		if seen[addr] {
			t.Fatalf("two concurrent bridge listeners were handed %s", addr)
		}
		seen[addr] = true
		port := portOfAddr(t, bridge.Addr())
		if port >= start && port <= end {
			t.Fatalf("concurrent bridge listener %s came from the ephemeral range %d-%d", addr, start, end)
		}
	}
}

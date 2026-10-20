// Package loopback allocates the loopback TCP ports that Gate listens on for
// dynamic, per-join work, and answers ownership questions about loopback
// addresses that Gate or the runtimes it manages publish.
//
// # Why one allocator
//
// A loopback port that is reserved with a listen-then-close (bind, read the
// port, close, tell a subprocess to bind it) is stealable for the whole window
// between the close and the child's bind: any socket the same host allocates in
// that window can take it. Gate's per-join translation bridge listener did
// exactly that (kanban t_40da126d, measured 2026-10-20): in an 8-way join burst
// it took the port the vialite supervisor had just reserved for a dynamic
// backend, the child died with "bind(..) failed with error(-98): Address already
// in use", readiness - which only asks whether something answers a dial - was
// satisfied by Gate's own listener, the dead backend was published anyway, and
// the join was routed into another session's tunnel while both stalled silently
// in configuration state. The same defect class returns with any new dynamic
// listener that draws from the kernel's ephemeral range, so every dynamically
// allocated loopback listener must come from a dedicated, non-ephemeral range
// and a reserved address must never be handed out twice.
//
// # Ranges
//
// The allocator never draws from the kernel's ephemeral range and refuses a
// range that overlaps it, because that is where every listen-then-close
// reservation on the host is drawn from. It also refuses a range that overlaps
// the range the vialite runtime reserves for dynamic backends
// (VialiteDynamicRange), so Gate's listeners and the runtime's reservations can
// never name the same port.
//
// # Adoption
//
// The package is deliberately outside pkg/internal so that moxy, which already
// depends on Gate, allocates its own dynamic loopback listeners from the same
// policy instead of open-coding net.Listen("tcp", "127.0.0.1:0").
package loopback

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	// ErrExhausted reports that no port in the reserved range could be bound.
	ErrExhausted = errors.New("loopback: no free port in the reserved range")
	// ErrRangeUnusable reports a range that cannot keep dynamically allocated
	// listeners out of another allocator's way.
	ErrRangeUnusable = errors.New("loopback: reserved port range is unusable")
)

const (
	// EnvRange overrides the range Gate's per-join bridge listeners are
	// allocated from, as "<start>-<end>".
	EnvRange = "GATE_LOOPBACK_PORT_RANGE"

	// BridgeRangeStart and BridgeRangeEnd bound Gate's own dynamically
	// allocated loopback listeners. The range sits below the Linux ephemeral
	// range (32768-60999 by default) and outside the range the vialite runtime
	// reserves for dynamic backends, so neither allocator can name a port the
	// other is about to use.
	BridgeRangeStart = 30000
	BridgeRangeEnd   = 31999

	// VialiteDynamicRangeStart and VialiteDynamicRangeEnd bound the loopback
	// addresses the vialite subprocess runner reserves for dynamic backends
	// (go/subprocess.go). Keep the two ranges disjoint: a listener in the
	// runtime's range can take the port a runtime reserved moments earlier.
	VialiteDynamicRangeStart = 20000
	VialiteDynamicRangeEnd   = 29999

	// defaultEphemeralStart and defaultEphemeralEnd are the Linux kernel
	// defaults, used when the host's own range cannot be read.
	defaultEphemeralStart = 32768
	defaultEphemeralEnd   = 60999
)

// Range is an inclusive port range.
type Range struct {
	Start int
	End   int
}

func (r Range) String() string { return fmt.Sprintf("%d-%d", r.Start, r.End) }

// Valid reports whether the range is an inclusive, unprivileged port range.
func (r Range) Valid() error {
	if r.Start < 1024 || r.End > 65535 || r.Start > r.End {
		return fmt.Errorf("%w: %s must be an inclusive range inside 1024-65535", ErrRangeUnusable, r)
	}
	return nil
}

// ContainsPort reports whether port is inside the range.
func (r Range) ContainsPort(port int) bool { return port >= r.Start && port <= r.End }

// ContainsAddress reports whether addr's port is inside the range.
func (r Range) ContainsAddress(addr string) bool {
	port, ok := PortOf(addr)
	return ok && r.ContainsPort(port)
}

// Overlaps reports whether the two ranges share a port.
func (r Range) Overlaps(other Range) bool {
	return r.Start <= other.End && other.Start <= r.End
}

// ParseRange parses "<start>-<end>".
func ParseRange(spec string) (Range, error) {
	spec = strings.TrimSpace(spec)
	start, end, ok := strings.Cut(spec, "-")
	if !ok {
		return Range{}, fmt.Errorf("%w: %q is not a <start>-<end> port range", ErrRangeUnusable, spec)
	}
	startPort, err := strconv.Atoi(strings.TrimSpace(start))
	if err != nil {
		return Range{}, fmt.Errorf("%w: %q start: %v", ErrRangeUnusable, spec, err)
	}
	endPort, err := strconv.Atoi(strings.TrimSpace(end))
	if err != nil {
		return Range{}, fmt.Errorf("%w: %q end: %v", ErrRangeUnusable, spec, err)
	}
	rng := Range{Start: startPort, End: endPort}
	if err := rng.Valid(); err != nil {
		return Range{}, err
	}
	return rng, nil
}

// BridgeRange returns the range Gate's own dynamically allocated loopback
// listeners are allocated from: BridgeRangeStart-BridgeRangeEnd, or the range
// named by EnvRange.
func BridgeRange() (Range, error) {
	if spec := strings.TrimSpace(os.Getenv(EnvRange)); spec != "" {
		return ParseRange(spec)
	}
	return Range{Start: BridgeRangeStart, End: BridgeRangeEnd}, nil
}

// VialiteDynamicRange returns the range the vialite runtime reserves dynamic
// backend addresses from.
func VialiteDynamicRange() Range {
	return Range{Start: VialiteDynamicRangeStart, End: VialiteDynamicRangeEnd}
}

// PortOf returns the port of a host:port address.
func PortOf(addr string) (int, bool) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, false
	}
	return port, true
}

// Allocator hands out loopback ports from one reserved range and never hands the
// same port out twice until it is released.
type Allocator struct {
	host string
	rng  Range

	mu     sync.Mutex
	taken  map[int]bool
	cursor int
}

// NewAllocator returns an allocator for rng on host.
//
// It fails closed on a range that cannot keep this allocator's listeners out of
// another allocator's way: a range inside the kernel's ephemeral range (where
// listen-then-close reservations are drawn from), a range overlapping the
// vialite runtime's dynamic-backend range, or a host that is not a loopback
// address (a per-join tunnel must never be reachable from outside the host).
func NewAllocator(rng Range, host string) (*Allocator, error) {
	if err := rng.Valid(); err != nil {
		return nil, err
	}
	if !isLoopbackHost(host) {
		return nil, fmt.Errorf("%w: host %q is not a loopback address; per-join listeners must never "+
			"be reachable from outside this host", ErrRangeUnusable, host)
	}
	if ephemeral := EphemeralRange(); rng.Overlaps(ephemeral) {
		return nil, fmt.Errorf("%w: %s overlaps the kernel ephemeral range %s, which is where "+
			"listen-then-close reservations are drawn from; set %s to a range outside it",
			ErrRangeUnusable, rng, ephemeral, EnvRange)
	}
	if dynamic := VialiteDynamicRange(); rng.Overlaps(dynamic) {
		return nil, fmt.Errorf("%w: %s overlaps the range the vialite runtime reserves for dynamic "+
			"backends (%s), so a listener here can take a runtime's reserved port",
			ErrRangeUnusable, rng, dynamic)
	}
	return &Allocator{host: host, rng: rng, taken: make(map[int]bool), cursor: rng.Start}, nil
}

// Range returns the allocator's port range.
func (a *Allocator) Range() Range { return a.rng }

// Host returns the address the allocator binds on.
func (a *Allocator) Host() string { return a.host }

// Listen binds and keeps a listener on a port from the allocator's range. The
// port stays reserved until the listener is closed.
func (a *Allocator) Listen() (net.Listener, error) {
	ln, port, err := a.bind()
	if err != nil {
		return nil, err
	}
	var once sync.Once
	return &reservedListener{Listener: ln, release: func() {
		once.Do(func() { a.release(port) })
	}}, nil
}

// Reserve binds a port from the allocator's range, closes the probe listener and
// keeps the port reserved for a consumer that binds it itself (a subprocess
// runtime). The returned release function must be called when the consumer is
// done with the address; it is safe to call more than once.
func (a *Allocator) Reserve() (addr string, release func(), err error) {
	ln, port, err := a.bind()
	if err != nil {
		return "", nil, err
	}
	addr = ln.Addr().String()
	closeErr := ln.Close()
	var once sync.Once
	release = func() { once.Do(func() { a.release(port) }) }
	if closeErr != nil {
		// The reservation stays: a probe listener that could not be closed is
		// still holding the port, so handing the address to two consumers would
		// be worse than leaking one port until the process exits.
		return "", release, fmt.Errorf("loopback: close probe listener on %s: %w", addr, closeErr)
	}
	return addr, release, nil
}

// HandedOut reports whether addr's port is currently reserved by this allocator,
// either by a live listener or by Reserve.
func (a *Allocator) HandedOut(addr string) bool {
	port, ok := PortOf(addr)
	if !ok {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.taken[port]
}

func (a *Allocator) bind() (net.Listener, int, error) {
	span := a.rng.End - a.rng.Start + 1
	a.mu.Lock()
	start := a.cursor
	a.mu.Unlock()
	for i := 0; i < span; i++ {
		port := a.rng.Start + ((start-a.rng.Start)+i)%span
		if !a.claim(port) {
			continue
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(a.host, strconv.Itoa(port)))
		if err != nil {
			a.release(port)
			continue
		}
		a.mu.Lock()
		a.cursor = a.rng.Start + ((port-a.rng.Start)+1)%span
		a.mu.Unlock()
		return ln, port, nil
	}
	return nil, 0, fmt.Errorf("%w: %s on %s", ErrExhausted, a.rng, a.host)
}

func (a *Allocator) claim(port int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.taken[port] {
		return false
	}
	a.taken[port] = true
	return true
}

func (a *Allocator) release(port int) {
	a.mu.Lock()
	delete(a.taken, port)
	a.mu.Unlock()
}

// reservedListener releases its port when closed.
type reservedListener struct {
	net.Listener
	release func()
}

func (l *reservedListener) Close() error {
	err := l.Listener.Close()
	l.release()
	return err
}

var (
	defaultOnce      sync.Once
	defaultAllocator *Allocator
	defaultErr       error
)

// Default returns the process-wide allocator for Gate's dynamically allocated
// loopback listeners. Every listener that lives for a join must come from it, so
// no two of them - and no address published for a runtime - can name the same
// port.
func Default() (*Allocator, error) {
	defaultOnce.Do(func() {
		rng, err := BridgeRange()
		if err != nil {
			defaultErr = err
			return
		}
		defaultAllocator, defaultErr = NewAllocator(rng, "127.0.0.1")
	})
	return defaultAllocator, defaultErr
}

// resetDefault forgets the process-wide allocator so a test can re-read EnvRange.
func resetDefault() {
	defaultOnce = sync.Once{}
	defaultAllocator = nil
	defaultErr = nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

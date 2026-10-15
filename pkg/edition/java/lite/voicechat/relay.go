package voicechat

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

const (
	magicByte    = 0xFF
	minPacket    = 17 // magic byte + 16 byte player UUID
	maxPacket    = 2048
	maxReplySize = 4096

	defaultIdleTimeout = 30 * time.Second
	defaultMaxFlows    = 1024
)

// RelayOptions tunes a Relay. Zero values select the defaults.
type RelayOptions struct {
	IdleTimeout time.Duration
	MaxFlows    int
}

func (o RelayOptions) withDefaults() RelayOptions {
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = defaultIdleTimeout
	}
	if o.MaxFlows <= 0 {
		o.MaxFlows = defaultMaxFlows
	}
	return o
}

// Relay forwards Simple Voice Chat datagrams between clients and the backends
// their TCP sessions are connected to. It never inspects encrypted payloads.
type Relay struct {
	conn     net.PacketConn
	sessions *Sessions
	opts     RelayOptions
	log      logr.Logger

	mu    sync.Mutex
	flows map[netip.AddrPort]*flow
}

// NewRelay creates a relay that serves datagrams arriving on conn.
func NewRelay(conn net.PacketConn, sessions *Sessions, opts RelayOptions, log logr.Logger) *Relay {
	return &Relay{
		conn:     conn,
		sessions: sessions,
		opts:     opts.withDefaults(),
		log:      log,
		flows:    map[netip.AddrPort]*flow{},
	}
}

// Serve relays until ctx is done or the socket fails. It closes the socket.
func (r *Relay) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = r.conn.Close() }()
	go r.reapLoop(ctx)
	defer r.closeAll()

	buf := make([]byte, maxPacket+1)
	for {
		n, from, err := r.conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		r.handleClient(from, buf[:n])
	}
}

func (r *Relay) handleClient(from net.Addr, pkt []byte) {
	if len(pkt) < minPacket || len(pkt) > maxPacket || pkt[0] != magicByte {
		return
	}
	ua, ok := from.(*net.UDPAddr)
	if !ok {
		return
	}
	ap := ua.AddrPort()
	ap = netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
	if f := r.flowFor(ap); f != nil {
		f.toBackends(pkt)
	}
}

func (r *Relay) flowFor(client netip.AddrPort) *flow {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.flows[client]; ok {
		return f
	}
	if len(r.flows) >= r.opts.MaxFlows {
		return nil
	}
	backends := r.sessions.Backends(client.Addr())
	if len(backends) == 0 {
		return nil // no TCP session for this IP: never an open relay
	}
	f := newFlow(r, client, backends)
	if f == nil {
		return nil
	}
	r.flows[client] = f
	r.log.V(1).Info("voice flow opened", "client", client.String(), "candidates", len(backends))
	return f
}

func (r *Relay) flowCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.flows)
}

func (r *Relay) reapLoop(ctx context.Context) {
	t := time.NewTicker(max(r.opts.IdleTimeout/2, 10*time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			r.sweep(now)
		}
	}
}

// sweep closes flows that are idle, whose client IP has no session left, or
// that are pinned to a backend the IP is no longer connected to.
func (r *Relay) sweep(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for client, f := range r.flows {
		if f.stale(now, r.opts.IdleTimeout, r.sessions.Backends(client.Addr())) {
			f.close()
			delete(r.flows, client)
		}
	}
}

func (r *Relay) closeAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for client, f := range r.flows {
		f.close()
		delete(r.flows, client)
	}
}

type upstream struct {
	conn    *net.UDPConn
	backend string
}

// flow is the relay state of one client ip:port.
type flow struct {
	relay  *Relay
	client netip.AddrPort
	last   atomic.Int64 // unix nanos of last traffic in either direction

	mu        sync.Mutex
	upstreams []*upstream
	pinned    *upstream
	closed    bool
}

// newFlow dials every candidate backend. It returns nil if none is reachable.
func newFlow(r *Relay, client netip.AddrPort, backends []string) *flow {
	f := &flow{relay: r, client: client}
	f.last.Store(time.Now().UnixNano())
	for _, b := range backends {
		raddr, err := net.ResolveUDPAddr("udp", b)
		if err != nil {
			r.log.V(1).Info("voice backend not resolvable", "backend", b, "error", err)
			continue
		}
		c, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			r.log.V(1).Info("voice backend dial failed", "backend", b, "error", err)
			continue
		}
		f.upstreams = append(f.upstreams, &upstream{conn: c, backend: b})
	}
	if len(f.upstreams) == 0 {
		return nil
	}
	for _, u := range f.upstreams {
		go f.readUpstream(u)
	}
	return f
}

func (f *flow) toBackends(pkt []byte) {
	f.last.Store(time.Now().UnixNano())
	f.mu.Lock()
	targets := f.upstreams
	if f.pinned != nil {
		targets = []*upstream{f.pinned}
	}
	f.mu.Unlock()
	for _, u := range targets {
		_, _ = u.conn.Write(pkt)
	}
}

func (f *flow) readUpstream(u *upstream) {
	buf := make([]byte, maxReplySize)
	to := net.UDPAddrFromAddrPort(f.client)
	for {
		n, err := u.conn.Read(buf)
		if err != nil {
			return
		}
		if !f.accept(u) {
			continue
		}
		f.last.Store(time.Now().UnixNano())
		_, _ = f.relay.conn.WriteTo(buf[:n], to)
	}
}

// accept pins the flow to u on its first reply; later replies only count for the pinned upstream.
func (f *flow) accept(u *upstream) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	if f.pinned == u {
		return true
	}
	if f.pinned != nil {
		return false
	}
	f.pinned = u
	for _, other := range f.upstreams {
		if other != u {
			_ = other.conn.Close()
		}
	}
	f.upstreams = []*upstream{u}
	return true
}

func (f *flow) stale(now time.Time, idle time.Duration, backends []string) bool {
	if len(backends) == 0 || now.Sub(time.Unix(0, f.last.Load())) > idle {
		return true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pinned != nil && !slices.Contains(backends, f.pinned.backend)
}

func (f *flow) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	for _, u := range f.upstreams {
		_ = u.conn.Close()
	}
}

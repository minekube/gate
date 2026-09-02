package lite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"
	"go.minekube.com/gate/pkg/edition/java/internal/protoutil"
	"go.minekube.com/gate/pkg/edition/java/lite/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/codec"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/gate/proto"
	connectiontelemetry "go.minekube.com/gate/pkg/telemetry/connection"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/netutil"
	"golang.org/x/sync/singleflight"
)

// IsConnectionRefused returns true if err indicates a connection refused error.
// These errors are common when backends are down and should use debug logging.
func IsConnectionRefused(err error) bool {
	return err != nil && (errors.Is(err, syscall.ECONNREFUSED) ||
		strings.Contains(strings.ToLower(err.Error()), "connection refused"))
}

// Forward forwards a client connection to a matching backend route.
func Forward(
	dialTimeout time.Duration,
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	pc *proto.PacketContext,
	strategyManager *StrategyManager,
) {
	defer func() { _ = client.Close() }()
	observation, observed := connectiontelemetry.FromContext(client.Context())

	log, src, route, routeHost, nextBackend, err := findRoute(routes, log, client, handshake, strategyManager)
	if err != nil {
		if observed {
			observation.Observe(client.Context(), connectiontelemetry.BackendStage, connectiontelemetry.BackendFailed)
		}
		// A player connection that matches no route is silently dropped, so log it at
		// the default verbosity: it is always an operator-actionable misconfiguration,
		// unlike the status pings that findRoute marks as debug-only.
		log.Info("failed to find route", "error", err)
		return
	}

	// Find a backend to dial successfully.
	if observed {
		observation.Observe(client.Context(), connectiontelemetry.BackendStage, connectiontelemetry.OutcomeUnknown)
	}
	backendAddr, log, dst, err := tryBackends(nextBackend, func(log logr.Logger, backendAddr string) (logr.Logger, net.Conn, error) {
		conn, err := dialRoute(client.Context(), dialTimeout, src.RemoteAddr(), route, backendAddr, handshake, pc, false)
		return log, conn, err
	})
	if err != nil {
		if observed {
			observation.Observe(client.Context(), connectiontelemetry.BackendStage, connectiontelemetry.BackendFailed)
		}
		return
	}
	defer func() { _ = dst.Close() }()

	if err = emptyReadBuff(client, dst); err != nil {
		if observed {
			observation.Observe(client.Context(), connectiontelemetry.Closed, connectiontelemetry.Failed)
		}
		errs.V(log, err).Info("failed to empty client buffer", "error", err)
		return
	}

	// Track connection for all strategies (used by status API and least-connections strategy)
	decrementConnection := strategyManager.TrackConnection(routeHost, backendAddr)
	defer decrementConnection()

	log.Info("forwarding connection", "backendAddr", backendAddr)
	if observed {
		observation.SetKind(connectiontelemetry.Gameplay)
		observation.Observe(client.Context(), connectiontelemetry.Play, connectiontelemetry.Success)
	}
	pipe(log, src, dst)
}

// errAllBackendsFailed is returned when all backends failed to dial.
var errAllBackendsFailed = errors.New("all backends failed")

// tryBackends tries backends until one succeeds or all fail.
func tryBackends[T any](next nextBackendFunc, try func(log logr.Logger, backendAddr string) (logr.Logger, T, error)) (string, logr.Logger, T, error) {
	for {
		backendAddr, log, ok := next()
		if !ok {
			var zero T
			return backendAddr, log, zero, errAllBackendsFailed
		}

		log, t, err := try(log, backendAddr)
		if err != nil {
			errs.V(log, err).Info("failed to try backend", "error", err)
			continue
		}
		return backendAddr, log, t, nil
	}
}

func emptyReadBuff(src netmc.MinecraftConn, dst net.Conn) error {
	buf, ok := src.(interface{ ReadBuffered() ([]byte, error) })
	if ok {
		b, err := buf.ReadBuffered()
		if err != nil {
			return fmt.Errorf("failed to read buffered bytes: %w", err)
		}
		if len(b) != 0 {
			_, err = dst.Write(b)
			if err != nil {
				return fmt.Errorf("failed to write buffered bytes: %w", err)
			}
		}
	}
	return nil
}

func pipe(log logr.Logger, src, dst net.Conn) {
	// disable deadlines
	var zero time.Time
	_ = src.SetDeadline(zero)
	_ = dst.SetDeadline(zero)

	type copyResult struct {
		direction string
		err       error
		bytes     int64
	}
	results := make(chan copyResult, 2)
	copyOne := func(direction string, to, from net.Conn) {
		n, err := io.Copy(to, from)
		if log.Enabled() {
			log.V(1).Info("done copying "+direction, "bytes", n, "error", err)
		}
		results <- copyResult{direction: direction, bytes: n, err: err}
	}
	go copyOne("client -> backend", dst, src)
	go copyOne("backend -> client", src, dst)

	first := <-results
	// EOF is a TCP half-close, not an instruction to discard bytes travelling in
	// the other direction. Propagate it with CloseWrite when supported, then
	// join both copy workers. An actual copy error makes the tunnel unusable, so
	// close both ends immediately to unblock its peer.
	if first.err == nil {
		var halfClosed bool
		if first.direction == "client -> backend" {
			halfClosed = closeWrite(dst)
		} else {
			halfClosed = closeWrite(src)
		}
		// net.Pipe and other non-TCP transports cannot half-close. Their only
		// safe completion policy is a full close so the peer copy can join.
		if !halfClosed {
			_ = src.Close()
			_ = dst.Close()
		}
	} else {
		_ = src.Close()
		_ = dst.Close()
	}
	second := <-results
	if second.err != nil {
		_ = src.Close()
		_ = dst.Close()
	}
	// A successful half-close has now propagated in both directions. Closing
	// releases transports which do not implement CloseWrite and guarantees no
	// worker survives this function.
	_ = src.Close()
	_ = dst.Close()
}

func closeWrite(conn net.Conn) bool {
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite() == nil
	}
	return false
}

type nextBackendFunc func() (backendAddr string, log logr.Logger, ok bool)

// substituteBackendParams replaces $1, $2, etc. in the backend address template with captured groups.
// If a parameter index is out of range or missing, it leaves the parameter as-is (e.g., "$99" stays "$99").
func substituteBackendParams(template string, groups []string) string {
	if len(groups) == 0 {
		return template
	}

	result := template
	// Replace $1, $2, etc. with captured groups
	// We need to handle this carefully to avoid replacing $10 when we mean $1
	// Process from highest index to lowest to avoid partial replacements
	for i := len(groups); i >= 1; i-- {
		param := fmt.Sprintf("$%d", i)
		if i-1 < len(groups) {
			result = strings.ReplaceAll(result, param, groups[i-1])
		}
	}
	return result
}

func findRoute(
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	strategyManager *StrategyManager,
) (
	newLog logr.Logger,
	src net.Conn,
	route *config.Route,
	routeHost string,
	nextBackend nextBackendFunc,
	err error,
) {
	srcConn, ok := netmc.Assert[interface{ Conn() net.Conn }](client)
	if !ok {
		return log, src, nil, "", nil, errors.New("failed to assert connection as net.Conn")
	}
	src = srcConn.Conn()

	clearedHost := ClearVirtualHost(handshake.ServerAddress)
	log = log.WithName("lite").WithValues(
		"clientAddr", netutil.Host(src.RemoteAddr()),
		"virtualHost", clearedHost,
		"protocol", proto.Protocol(handshake.ProtocolVersion).String(),
	)

	host, route, groups := FindRouteWithGroups(clearedHost, routes...)
	if route == nil {
		// Status pings hit unknown hosts constantly, so they keep this out of the
		// default log via errs.V. Forward logs it unconditionally for players.
		return log, src, nil, "", nil, &errs.VerbosityError{
			Err:       fmt.Errorf("no route configured for host %s", clearedHost),
			Verbosity: 1,
		}
	}
	log = log.WithValues("route", host)

	if len(route.Backend) == 0 {
		return log, src, route, host, nil, errors.New("no backend configured for route")
	}

	tryBackends := route.Backend.Copy()
	for i := range tryBackends {
		tryBackends[i] = substituteBackendParams(tryBackends[i], groups)
	}
	nextBackend = func() (string, logr.Logger, bool) {
		if len(tryBackends) == 0 {
			return "", log, false
		}

		// Always use strategy manager (it handles empty strategy as sequential default)
		backendAddr, newLog, ok := strategyManager.GetNextBackend(log, route, host, tryBackends)
		if !ok {
			return "", log, false
		}

		// Remove selected backend from list to avoid retrying it
		for i, backend := range tryBackends {
			normalizedBackend, err := netutil.Parse(backend, src.RemoteAddr().Network())
			if err != nil {
				continue
			}
			normalizedAddr := normalizedBackend.String()
			if _, port := netutil.HostPort(normalizedBackend); port == 0 {
				normalizedAddr = net.JoinHostPort(normalizedBackend.String(), "25565")
			}

			normalizedSelected, err := netutil.Parse(backendAddr, src.RemoteAddr().Network())
			if err != nil {
				continue
			}
			selectedAddr := normalizedSelected.String()
			if _, port := netutil.HostPort(normalizedSelected); port == 0 {
				selectedAddr = net.JoinHostPort(normalizedSelected.String(), "25565")
			}

			if normalizedAddr == selectedAddr {
				tryBackends = append(tryBackends[:i], tryBackends[i+1:]...)
				break
			}
		}

		return backendAddr, newLog.WithValues("backendAddr", backendAddr), true
	}

	return log, src, route, host, nextBackend, nil
}

func dialRoute(
	ctx context.Context,
	dialTimeout time.Duration,
	srcAddr net.Addr,
	route *config.Route,
	backendAddr string,
	handshake *packet.Handshake,
	handshakeCtx *proto.PacketContext,
	forceUpdatePacketContext bool,
) (dst net.Conn, err error) {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	var dialer net.Dialer
	dst, err = dialer.DialContext(dialCtx, "tcp", backendAddr)
	if err != nil {
		v := 0
		if dialCtx.Err() != nil {
			v++
		}
		// Treat connection refused as debug level to reduce spam
		// These are common when backends are down and should not flood logs
		if IsConnectionRefused(err) {
			v = 1
		}
		return nil, &errs.VerbosityError{
			Verbosity: v,
			Err:       fmt.Errorf("failed to connect to backend %s: %w", backendAddr, err),
		}
	}
	dstConn := dst
	defer func() {
		if err != nil {
			_ = dstConn.Close()
		}
	}()

	if route.ProxyProtocol {
		header := protoutil.ProxyHeader(srcAddr, dst.RemoteAddr())
		if _, err = header.WriteTo(dst); err != nil {
			return dst, fmt.Errorf("failed to write proxy protocol header to backend: %w", err)
		}
	}

	if route.ModifyVirtualHost {
		clearedHost := ClearVirtualHost(handshake.ServerAddress)
		backendHost := netutil.HostStr(backendAddr)
		if !strings.EqualFold(clearedHost, backendHost) {
			// Modify the handshake packet to use the backend host as virtual host.
			handshake.ServerAddress = strings.ReplaceAll(handshake.ServerAddress, clearedHost, backendHost)
			forceUpdatePacketContext = true
		}
	}
	if route.GetTCPShieldRealIP() && IsTCPShieldRealIP(handshake.ServerAddress) {
		// Modify the handshake packet to use TCPShieldRealIP of the client.
		handshake.ServerAddress = TCPShieldRealIP(handshake.ServerAddress, srcAddr)
		forceUpdatePacketContext = true
	}
	if forceUpdatePacketContext {
		update(handshakeCtx, handshake)
	}

	// Forward handshake packet as is.
	if err = writePacket(dst, handshakeCtx); err != nil {
		return dst, fmt.Errorf("failed to write handshake packet to backend: %w", err)
	}

	return dst, nil
}

func writePacket(dst net.Conn, pc *proto.PacketContext) error {
	err := util.WriteVarInt(dst, len(pc.Payload))
	if err != nil {
		return fmt.Errorf("failed to write packet length: %w", err)
	}
	_, err = dst.Write(pc.Payload)
	if err != nil {
		return fmt.Errorf("failed to write packet payload: %w", err)
	}
	return nil
}

func update(pc *proto.PacketContext, h *packet.Handshake) {
	payload := new(bytes.Buffer)
	_ = util.WriteVarInt(payload, int(pc.PacketID))
	_ = h.Encode(pc, payload)
	pc.Payload = payload.Bytes()
}

// ResolveStatusResponse resolves the status response for the matching route and caches it for a short time.
func ResolveStatusResponse(
	dialTimeout time.Duration,
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	handshakeCtx *proto.PacketContext,
	statusRequestCtx *proto.PacketContext,
	strategyManager *StrategyManager,
) (logr.Logger, *packet.StatusResponse, error) {
	return ResolveStatusResponseWithGeneration(dialTimeout, 0, routes, log, client, handshake, handshakeCtx, statusRequestCtx, strategyManager)
}

// ResolveStatusResponseWithGeneration resolves a status response with a route snapshot generation.
func ResolveStatusResponseWithGeneration(
	dialTimeout time.Duration,
	routeGeneration uint64,
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	handshakeCtx *proto.PacketContext,
	statusRequestCtx *proto.PacketContext,
	strategyManager *StrategyManager,
) (logr.Logger, *packet.StatusResponse, error) {
	log, src, route, _, nextBackend, err := findRoute(routes, log, client, handshake, strategyManager)
	if err != nil {
		return log, nil, err
	}

	_, log, res, err := tryBackends(nextBackend, func(log logr.Logger, backendAddr string) (logr.Logger, *packet.StatusResponse, error) {
		// Measure status response time for latency tracking (better than dial time)
		start := time.Now()
		newLog, response, respErr := resolveStatusResponse(src, dialTimeout, routeGeneration, backendAddr, route, log, client, handshake, handshakeCtx, statusRequestCtx)
		statusLatency := time.Since(start)

		// Record latency for lowest-latency strategy (only on success)
		if respErr == nil {
			strategyManager.RecordLatency(backendAddr, statusLatency)
		}

		return newLog, response, respErr
	})

	// Handle fallback if all backends failed
	if err != nil {
		fallbackResp, fallbackLog := handleFallbackResponse(log, route, handshakeCtx.Protocol, err)
		if fallbackResp != nil {
			return fallbackLog, fallbackResp, nil
		}
	}

	return log, res, err
}

// handleFallbackResponse handles the fallback response when all backends fail.
// This is extracted for better testability.
func handleFallbackResponse(log logr.Logger, route *config.Route, protocol proto.Protocol, backendErr error) (*packet.StatusResponse, logr.Logger) {
	if route == nil || route.Fallback == nil {
		return nil, log
	}

	log.Info("failed to resolve status response, will use fallback status response", "error", backendErr)

	// Fallback status response if configured
	fallbackPong, err := route.Fallback.Response(protocol)
	if err != nil {
		log.Info("failed to get fallback status response", "error", err)
		return nil, log
	}

	if fallbackPong != nil {
		status, err2 := json.Marshal(fallbackPong)
		if err2 != nil {
			log.Error(err2, "failed to marshal fallback status response")
			return nil, log
		}
		if log.V(1).Enabled() {
			log.V(1).Info("using fallback status response", "status", string(status))
		}
		return &packet.StatusResponse{Status: string(status)}, log
	}

	return nil, log
}

var pingCache = newPingStatusCache(time.Now, new(singleflight.Group))

// ResetPingCache clears cached ping results and prevents in-flight loads from repopulating them.
func ResetPingCache() {
	pingCache.reset()
	compiledRegexCache.DeleteAll()
}

func init() {
	go pingCache.cache.Start() // start ttl eviction once
}

type pingKey struct {
	backendAddr     string
	protocol        proto.Protocol
	routeGeneration uint64
}

type pingResult struct {
	res *packet.StatusResponse
	err error
}

type flightGroup interface {
	DoChan(string, func() (any, error)) <-chan singleflight.Result
}

type pingStatusCache struct {
	mu         sync.Mutex
	cache      *ttlcache.Cache[pingKey, *pingResult]
	group      flightGroup
	now        func() time.Time
	generation uint64
}

func newPingStatusCache(now func() time.Time, group flightGroup) *pingStatusCache {
	return &pingStatusCache{
		cache: ttlcache.New[pingKey, *pingResult](ttlcache.WithDisableTouchOnHit[pingKey, *pingResult]()),
		group: group,
		now:   now,
	}
}

func (c *pingStatusCache) get(key pingKey) *pingResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getLocked(key)
}

func (c *pingStatusCache) getLocked(key pingKey) *pingResult {
	item := c.cache.Get(key)
	if item == nil {
		return nil
	}
	expiresAt := item.ExpiresAt()
	if !expiresAt.IsZero() && !c.now().Before(expiresAt) {
		c.cache.Delete(key)
		return nil
	}
	return item.Value()
}

func (c *pingStatusCache) load(key pingKey, ttl time.Duration, load func() *pingResult) *pingResult {
	c.mu.Lock()
	generation := c.generation
	if result := c.getLocked(key); result != nil {
		c.mu.Unlock()
		return result
	}
	c.mu.Unlock()

	flightKey := fmt.Sprintf("%d:%d:%s:%d", generation, key.routeGeneration, key.backendAddr, key.protocol)
	result := <-c.group.DoChan(flightKey, func() (any, error) {
		c.mu.Lock()
		if generation == c.generation {
			if cached := c.getLocked(key); cached != nil {
				c.mu.Unlock()
				return cached, nil
			}
		}
		c.mu.Unlock()

		loaded := load()
		c.mu.Lock()
		if generation == c.generation {
			c.cache.Set(key, loaded, ttl)
		}
		c.mu.Unlock()
		return loaded, nil
	})
	return result.Val.(*pingResult)
}

func (c *pingStatusCache) reset() {
	c.mu.Lock()
	c.generation++
	c.cache.DeleteAll()
	c.mu.Unlock()
}

func resolveStatusResponse(
	src net.Conn,
	dialTimeout time.Duration,
	routeGeneration uint64,
	backendAddr string,
	route *config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	handshakeCtx *proto.PacketContext,
	statusRequestCtx *proto.PacketContext,
) (logr.Logger, *packet.StatusResponse, error) {
	key := pingKey{backendAddr, proto.Protocol(handshake.ProtocolVersion), routeGeneration}

	// fast path: use cache without loader
	if route.CachePingEnabled() {
		val := pingCache.get(key)
		if val != nil {
			log.V(1).Info("returning cached status result")
			return log, val.res, val.err
		}
	}

	// slow path: load cache, block many requests to same route
	//
	// resolve ping of remote backend, cache and return it.
	// if more ping requests arrive at slow path for the same route
	// the ping result of the first original request is returned to
	// ensure a single connection per route for fetching the status
	// while allowing many ping requests

	load := func(ctx context.Context) (*packet.StatusResponse, error) {
		log.V(1).Info("resolving status")

		ctx = logr.NewContext(ctx, log)
		dst, err := dialRoute(ctx, dialTimeout, src.RemoteAddr(), route, backendAddr, handshake, handshakeCtx, route.CachePingEnabled())
		if err != nil {
			return nil, fmt.Errorf("failed to dial route: %w", err)
		}
		defer func() { _ = dst.Close() }()

		log = log.WithValues("backendAddr", netutil.Host(dst.RemoteAddr()))
		return fetchStatus(log, dst, handshakeCtx.Protocol, statusRequestCtx)
	}

	if !route.CachePingEnabled() {
		res, err := load(client.Context())
		return log, res, err
	}

	loadResult := func() *pingResult {
		res, err := load(context.Background())
		return &pingResult{res: res, err: err}
	}

	resultChan := make(chan *pingResult, 1)
	go func() { resultChan <- pingCache.load(key, route.GetCachePingTTL(), loadResult) }()

	select {
	case result := <-resultChan:
		return log, result.res, result.err
	case <-client.Context().Done():
		return log, nil, &errs.VerbosityError{
			Err:       context.Cause(client.Context()),
			Verbosity: 1,
		}
	}
}

func fetchStatus(
	log logr.Logger,
	conn net.Conn,
	protocol proto.Protocol,
	statusRequestCtx *proto.PacketContext,
) (*packet.StatusResponse, error) {
	if err := writePacket(conn, statusRequestCtx); err != nil {
		return nil, fmt.Errorf("failed to write status request packet to backend: %w", err)
	}

	dec := codec.NewDecoder(conn, proto.ClientBound, log.V(2))
	dec.SetProtocol(protocol)
	dec.SetState(state.Status)

	return decodeStatusResponse(dec)
}

// statusDecoder interface for decoding status responses (allows mocking in tests)
type statusDecoder interface {
	Decode() (*proto.PacketContext, error)
}

// decodeStatusResponse decodes a status response from the decoder, handling the
// ErrDecoderLeftBytes error that can occur when mods like BetterCompatibilityChecker
// add extra data to the status response packet.
func decodeStatusResponse(dec statusDecoder) (*packet.StatusResponse, error) {
	pongCtx, err := dec.Decode()
	if err != nil && !errors.Is(err, proto.ErrDecoderLeftBytes) {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}
	// If we got ErrDecoderLeftBytes, pongCtx should still be valid
	if pongCtx == nil {
		return nil, fmt.Errorf("failed to decode status response: got nil packet context")
	}

	res, ok := pongCtx.Packet.(*packet.StatusResponse)
	if !ok {
		return nil, fmt.Errorf("received unexpected response: %s, expected %T", pongCtx, res)
	}

	return res, nil
}

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

	log, match, err := findRoute(routes, log, client, handshake, strategyManager)
	if err != nil {
		errs.V(log, err).Info("failed to find route", "error", err)
		return
	}

	src := match.src
	route := match.route
	rt := strategyManager.runtime
	observe := rt != nil && rt.maybeEnableObservability()
	clientAddr := cloneAddr(src.RemoteAddr())
	clientIP := effectiveClientIP(route, handshake.ServerAddress, src.RemoteAddr())

	// Find a backend to dial successfully.
	backendAddr, log, dst, err := tryBackends(match.nextBackend, func(log logr.Logger, backendAddr string) (logr.Logger, net.Conn, error) {
		conn, err := dialRoute(client.Context(), dialTimeout, src.RemoteAddr(), route, backendAddr, handshake, pc, false)
		return log, conn, err
	})
	if err != nil {
		if observe {
			rt.Event().Fire(&ForwardEndedEvent{
				ConnectionID: "",
				ClientIP:     clientIP,
				ClientAddr:   clientAddr,
				BackendAddr:  nil,
				Host:         match.virtualHost,
				RouteID:      match.routeID(),
				StartedAt:    time.Time{},
				EndedAt:      time.Now(),
				Reason:       BackendConnectFailed,
			})
		}
		return
	}
	defer func() { _ = dst.Close() }()

	if err = emptyReadBuff(client, dst); err != nil {
		errs.V(log, err).Info("failed to empty client buffer", "error", err)
		return
	}

	// Track connection for least-connections strategy
	var decrementConnection func()
	if route.Strategy == config.StrategyLeastConnections {
		decrementConnection = strategyManager.IncrementConnection(backendAddr)
		defer decrementConnection()
	}

	var connectionID string
	var startedAt time.Time
	if observe {
		connectionID = rt.tracker.nextConnectionID()
		startedAt = time.Now()
		active := ActiveForward{
			ConnectionID: connectionID,
			ClientIP:     clientIP,
			BackendAddr:  cloneAddr(dst.RemoteAddr()),
			Host:         match.virtualHost,
			RouteID:      match.routeID(),
			StartedAt:    startedAt,
		}
		rt.tracker.add(active)
		rt.Event().Fire(&ForwardStartedEvent{
			ConnectionID: active.ConnectionID,
			ClientIP:     active.ClientIP,
			ClientAddr:   cloneAddr(clientAddr),
			BackendAddr:  cloneAddr(active.BackendAddr),
			Host:         active.Host,
			RouteID:      active.RouteID,
			StartedAt:    active.StartedAt,
		})
	}

	log.Info("forwarding connection", "backendAddr", backendAddr)
	reason, endedAt := pipeWithReason(client.Context(), log, src, dst)
	if observe {
		rt.tracker.remove(connectionID)
		rt.Event().Fire(&ForwardEndedEvent{
			ConnectionID: connectionID,
			ClientIP:     clientIP,
			ClientAddr:   cloneAddr(clientAddr),
			BackendAddr:  cloneAddr(dst.RemoteAddr()),
			Host:         match.virtualHost,
			RouteID:      match.routeID(),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			Reason:       reason,
		})
	}
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

type pipeDirection int

const (
	pipeClientToBackend pipeDirection = iota
	pipeBackendToClient
)

type copyResult struct {
	dir   pipeDirection
	bytes int64
	err   error
}

func pipeWithReason(ctx context.Context, log logr.Logger, src, dst net.Conn) (ForwardEndReason, time.Time) {
	// disable deadlines
	var zero time.Time
	_ = src.SetDeadline(zero)
	_ = dst.SetDeadline(zero)

	results := make(chan copyResult, 2)

	go func() {
		i, err := io.Copy(src, dst)
		if log.Enabled() {
			log.V(1).Info("done copying backend -> client", "bytes", i, "error", err)
		}
		results <- copyResult{dir: pipeBackendToClient, bytes: i, err: err}
	}()

	go func() {
		i, err := io.Copy(dst, src)
		if log.Enabled() {
			log.V(1).Info("done copying client -> backend", "bytes", i, "error", err)
		}
		results <- copyResult{dir: pipeClientToBackend, bytes: i, err: err}
	}()

	first := <-results
	endedAt := time.Now()

	_ = src.Close()
	_ = dst.Close()

	return classifyForwardEnd(ctx, first), endedAt
}

type nextBackendFunc func() (backendAddr string, log logr.Logger, ok bool)

func classifyForwardEnd(ctx context.Context, result copyResult) ForwardEndReason {
	if result.err != nil {
		var netErr net.Error
		if errors.As(result.err, &netErr) && netErr.Timeout() {
			return Timeout
		}
		if ctx != nil && ctx.Err() != nil {
			return Shutdown
		}
		return Error
	}

	if result.dir == pipeBackendToClient {
		return BackendClosed
	}
	return ClientClosed
}

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
	match routeMatch,
	err error,
) {
	srcConn, ok := netmc.Assert[interface{ Conn() net.Conn }](client)
	if !ok {
		return log, match, errors.New("failed to assert connection as net.Conn")
	}
	match.src = srcConn.Conn()

	clearedHost := ClearVirtualHost(handshake.ServerAddress)
	log = log.WithName("lite").WithValues(
		"clientAddr", netutil.Host(match.src.RemoteAddr()),
		"virtualHost", clearedHost,
		"protocol", proto.Protocol(handshake.ProtocolVersion).String(),
	)
	match.virtualHost = clearedHost

	var (
		route      *config.Route
		host       string
		groups     []string
		routeIndex = -1
	)
	for i := range routes {
		rt := &routes[i]
		for _, candidate := range rt.Host {
			matched, capturedGroups := matchWithGroups(clearedHost, candidate)
			if matched {
				route = rt
				host = candidate
				groups = capturedGroups
				routeIndex = i
				break
			}
		}
		if route != nil {
			break
		}
	}
	if route == nil {
		return log.V(1), match, fmt.Errorf("no route configured for host %s", clearedHost)
	}
	log = log.WithValues("route", host)
	match.route = route
	match.routeIndex = routeIndex
	match.matchedHost = host

	if len(route.Backend) == 0 {
		return log, match, errors.New("no backend configured for route")
	}

	tryBackends := route.Backend.Copy()
	match.nextBackend = func() (string, logr.Logger, bool) {
		if len(tryBackends) == 0 {
			return "", log, false
		}

		// Always use strategy manager (it handles empty strategy as sequential default)
		backendAddr, newLog, ok := strategyManager.GetNextBackend(log, route, host, tryBackends)
		if !ok {
			return "", log, false
		}

		// Substitute parameters in backend address if groups were captured
		if len(groups) > 0 {
			backendAddr = substituteBackendParams(backendAddr, groups)
		}

		// Remove selected backend from list to avoid retrying it
		for i, backend := range tryBackends {
			// Apply parameter substitution to the original backend for comparison
			originalBackend := backend
			if len(groups) > 0 {
				originalBackend = substituteBackendParams(backend, groups)
			}

			normalizedBackend, err := netutil.Parse(originalBackend, match.src.RemoteAddr().Network())
			if err != nil {
				continue
			}
			normalizedAddr := normalizedBackend.String()
			if _, port := netutil.HostPort(normalizedBackend); port == 0 {
				normalizedAddr = net.JoinHostPort(normalizedBackend.String(), "25565")
			}

			normalizedSelected, err := netutil.Parse(backendAddr, match.src.RemoteAddr().Network())
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

	return log, match, nil
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
	log, match, err := findRoute(routes, log, client, handshake, strategyManager)
	if err != nil {
		return log, nil, err
	}
	src := match.src
	route := match.route

	_, log, res, err := tryBackends(match.nextBackend, func(log logr.Logger, backendAddr string) (logr.Logger, *packet.StatusResponse, error) {
		// Measure status response time for latency tracking (better than dial time)
		start := time.Now()
		newLog, response, respErr := resolveStatusResponse(src, dialTimeout, backendAddr, route, log, client, handshake, handshakeCtx, statusRequestCtx)
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

type routeMatch struct {
	src         net.Conn
	route       *config.Route
	routeIndex  int
	matchedHost string
	virtualHost string
	nextBackend nextBackendFunc
}

func (m routeMatch) routeID() string {
	if m.routeIndex < 0 {
		return m.matchedHost
	}
	return fmt.Sprintf("%d:%s", m.routeIndex, m.matchedHost)
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

var (
	pingCache = ttlcache.New[pingKey, *pingResult]()
	sfg       = new(singleflight.Group)
)

// ResetPingCache resets the ping cache.
func ResetPingCache() {
	pingCache.DeleteAll()
	compiledRegexCache.DeleteAll()
}

func init() {
	go pingCache.Start() // start ttl eviction once
}

type pingKey struct {
	backendAddr string
	protocol    proto.Protocol
}

type pingResult struct {
	res *packet.StatusResponse
	err error
}

func resolveStatusResponse(
	src net.Conn,
	dialTimeout time.Duration,
	backendAddr string,
	route *config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	handshakeCtx *proto.PacketContext,
	statusRequestCtx *proto.PacketContext,
) (logr.Logger, *packet.StatusResponse, error) {
	key := pingKey{backendAddr, proto.Protocol(handshake.ProtocolVersion)}

	// fast path: use cache without loader
	if route.CachePingEnabled() {
		item := pingCache.Get(key)
		if item != nil {
			log.V(1).Info("returning cached status result")
			val := item.Value()
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

	opt := withLoader(sfg, route.GetCachePingTTL(), func(key pingKey) *pingResult {
		res, err := load(context.Background())
		return &pingResult{res: res, err: err}
	})

	resultChan := make(chan *pingResult, 1)
	go func() { resultChan <- pingCache.Get(key, opt).Value() }()

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

// withLoader returns a ttlcache option that uses the given load function to load a value for a key
// if it is not already cached.
func withLoader[K comparable, V any](group *singleflight.Group, ttl time.Duration, load func(key K) V) ttlcache.Option[K, V] {
	loader := ttlcache.LoaderFunc[K, V](
		func(c *ttlcache.Cache[K, V], key K) *ttlcache.Item[K, V] {
			v := load(key)
			return c.Set(key, v, ttl)
		},
	)
	return ttlcache.WithLoader[K, V](
		ttlcache.NewSuppressedLoader[K, V](loader, group),
	)
}

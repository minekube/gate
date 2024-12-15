package lite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"
	"go.minekube.com/gate/pkg/edition/java/internal/protoutil"
	"go.minekube.com/gate/pkg/edition/java/lite/blacklist"
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

var (
	globalBlacklist        *blacklist.Blacklist
	routeBlacklist         *blacklist.RouteBlacklist
	connectionCountManager *ConnectionCountManager
	logger                 logr.Logger
	watcher                *fsnotify.Watcher
)

// ConnectionCountManager manages connection counts for each route
type ConnectionCountManager struct {
	counts map[string]int
	mu     sync.Mutex
}

// NewConnectionCountManager creates a new ConnectionCountManager
func NewConnectionCountManager() *ConnectionCountManager {
	return &ConnectionCountManager{
		counts: make(map[string]int),
	}
}

// Increment increases the connection count for a route
func (cm *ConnectionCountManager) Increment(routeKey string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.counts[routeKey]++
}

// Decrement decreases the connection count for a route
func (cm *ConnectionCountManager) Decrement(routeKey string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.counts[routeKey] > 0 {
		cm.counts[routeKey]--
	}
}

// GetCount returns the current connection count for a route
func (cm *ConnectionCountManager) GetCount(routeKey string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.counts[routeKey]
}

// Add this helper function at the top level
func isIPBlacklisted(ip string, route *config.Route) bool {
	if globalBlacklist != nil && globalBlacklist.Contains(ip) {
		return true
	}
	if routeBlacklist != nil && routeBlacklist.Contains(route.Host[0], ip) {
		return true
	}
	for _, blacklistedIP := range route.Blacklist {
		if ip == blacklistedIP {
			return true
		}
	}
	return false
}

// InitBlacklist initializes the global blacklist and sets up a file watcher
func InitBlacklist(globalBlacklistPath, routeBlacklistPath string) error {
	var err error
	globalBlacklist, err = blacklist.NewBlacklist(globalBlacklistPath)
	if err != nil {
		return fmt.Errorf("failed to initialize global blacklist: %w", err)
	}

	routeBlacklist, err = blacklist.NewRouteBlacklist(routeBlacklistPath)
	if err != nil {
		return fmt.Errorf("failed to initialize route blacklist: %w", err)
	}

	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	err = watcher.Add(globalBlacklistPath)
	if err != nil {
		return fmt.Errorf("failed to add global blacklist file to watcher: %w", err)
	}

	err = watcher.Add(routeBlacklistPath)
	if err != nil {
		return fmt.Errorf("failed to add route blacklist file to watcher: %w", err)
	}

	go watchBlacklistFiles(globalBlacklistPath, routeBlacklistPath)

	return nil
}

func watchBlacklistFiles(globalBlacklistPath, routeBlacklistPath string) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				switch event.Name {
				case globalBlacklistPath:
					logger.Info("Global blacklist file modified, reloading...")
					err := globalBlacklist.Load()
					if err != nil {
						logger.Error(err, "Failed to reload global blacklist")
					} else {
						logger.Info("Global blacklist reloaded successfully")
					}
				case routeBlacklistPath:
					logger.Info("Route blacklist file modified, reloading...")
					err := routeBlacklist.Load()
					if err != nil {
						logger.Error(err, "Failed to reload route blacklist")
					} else {
						logger.Info("Route blacklist reloaded successfully")
					}
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Error(err, "Error watching blacklist files")
		}
	}
}

// Forward forwards a client connection to a matching backend route.
func Forward(
	dialTimeout time.Duration,
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
	pc *proto.PacketContext,
) {
	defer func() { _ = client.Close() }()

	log, src, route, nextBackend, err := findRoute(routes, log, client, handshake)
	if err != nil {
		errs.V(log, err).Info("failed to find route", "error", err)
		return
	}

	// Get client IP
	clientIP, _, err := net.SplitHostPort(src.RemoteAddr().String())
	if err != nil {
		log.Error(err, "failed to parse client IP address")
		return
	}

	// Check if the IP is blacklisted
	if isIPBlacklisted(clientIP, route) {
		log.Info("connection denied due to blacklisted IP",
			"clientIP", clientIP,
			"route", route.Host[0])
		return
	}

	// Check if the route has reached its max connections
	routeKey := route.Host[0] // Use first host as the key
	if route.MaxConnections > 0 {
		currentCount := connectionCountManager.GetCount(routeKey)
		if currentCount >= route.MaxConnections {
			log.Info("connection denied due to max connections reached",
				"route", routeKey,
				"maxConnections", route.MaxConnections,
				"currentConnections", currentCount)
			return
		}
	}

	// Find a backend to dial successfully.
	log, dst, err := tryBackends(log, nextBackend, func(log logr.Logger, backendAddr string) (logr.Logger, net.Conn, error) {
		conn, err := dialRoute(client.Context(), dialTimeout, src.RemoteAddr(), route, backendAddr, handshake, pc, false)
		return log, conn, err
	})
	if err != nil {
		log.Error(err, "Failed to connect to any backend")
		return
	}

	// Increment the connection count only after successful connection
	if route.MaxConnections > 0 {
		connectionCountManager.Increment(routeKey)
		defer connectionCountManager.Decrement(routeKey)
	}

	defer func() { _ = dst.Close() }()

	if err = emptyReadBuff(client, dst); err != nil {
		errs.V(log, err).Info("failed to empty client buffer", "error", err)
		return
	}

	// Extract the backendAddr with the port and the IP separately
	backendAddrWithPort := dst.RemoteAddr().String()
	backendIP, _, _ := net.SplitHostPort(backendAddrWithPort)

	// Include the strategy name in the log
	strategyName := route.Strategy

	log.Info("forwarding connection",
		"clientAddr", netutil.Host(src.RemoteAddr()),
		"virtualHost", ClearVirtualHost(handshake.ServerAddress),
		"protocol", proto.Protocol(handshake.ProtocolVersion).String(),
		"route", routeKey,
		"backendAddr", backendAddrWithPort,
		"backendIP", backendIP,
		"strategy", strategyName,
		"currentConnections", connectionCountManager.GetCount(routeKey))

	pipe(log, src, dst)
}

// errAllBackendsFailed is returned when all backends failed to dial.
var errAllBackendsFailed = errors.New("all backends failed")

// tryBackends tries backends until one succeeds or all fail.
func tryBackends[T any](log logr.Logger, next nextBackendFunc, try func(log logr.Logger, backendAddr string) (logr.Logger, T, error)) (logr.Logger, T, error) {
	var lastErr error
	for {
		backendAddr, ok := next()
		if !ok {
			var zero T
			if lastErr != nil {
				return log, zero, fmt.Errorf("all backends failed, last error: %w", lastErr)
			}
			return log, zero, errAllBackendsFailed
		}

		log, t, err := try(log, backendAddr)
		if err == nil {
			return log, t, nil
		}
		lastErr = err
		log.V(1).Info("Backend connection attempt failed", "backendAddr", backendAddr, "error", err)
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

	go func() {
		i, err := io.Copy(src, dst)
		if log.Enabled() {
			log.V(1).Info("done copying backend -> client", "bytes", i, "error", err)
		}
	}()
	i, err := io.Copy(dst, src)
	if log.Enabled() {
		log.V(1).Info("done copying client -> backend", "bytes", i, "error", err)
	}
}

type nextBackendFunc func() (backendAddr string, ok bool)

func findRoute(
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
) (
	newLog logr.Logger,
	src net.Conn,
	route *config.Route,
	nextBackend nextBackendFunc,
	err error,
) {
	srcConn, ok := netmc.Assert[interface{ Conn() net.Conn }](client)
	if !ok {
		return log, src, nil, nil, errors.New("failed to assert connection as net.Conn")
	}
	src = srcConn.Conn()

	clearedHost := ClearVirtualHost(handshake.ServerAddress)
	log = log.WithName("lite").WithValues(
		"clientAddr", netutil.Host(src.RemoteAddr()),
		"virtualHost", clearedHost,
		"protocol", proto.Protocol(handshake.ProtocolVersion).String(),
	)

	host, route := FindRoute(clearedHost, routes...)
	if route == nil {
		return log.V(1), src, nil, nil, fmt.Errorf("no route configured for host %s", clearedHost)
	}
	log = log.WithValues("route", host)

	if len(route.Backend) == 0 {
		return log, src, route, nil, errors.New("no backend configured for route")
	}

	tryBackends := route.Backend.Copy()
	nextBackend = func() (string, bool) {
		switch route.Strategy {
		case "random":
			return randomNextBackend(tryBackends)()
		case "round-robin":
			return roundRobinNextBackend(host, tryBackends)()
		case "least connections":
			return leastConnectionsNextBackend(tryBackends)()
		case "lowest latency":
			return lowestLatencyNextBackend(tryBackends)()
		default:
			// Default to random strategy
			return randomNextBackend(tryBackends)()
		}
	}

	return log, src, route, nextBackend, nil
}

func randomNextBackend(tryBackends []string) nextBackendFunc {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return func() (string, bool) {
		if len(tryBackends) == 0 {
			return "", false
		}
		randIndex := r.Intn(len(tryBackends))
		return tryBackends[randIndex], true
	}
}

var roundRobinIndex = make(map[string]int)

func roundRobinNextBackend(routeHost string, tryBackends []string) nextBackendFunc {
	return func() (string, bool) {
		if len(tryBackends) == 0 {
			return "", false
		}
		index := roundRobinIndex[routeHost] % len(tryBackends)
		backend := tryBackends[index]
		roundRobinIndex[routeHost]++
		return backend, true
	}
}


func leastConnectionsNextBackend(tryBackends []string) nextBackendFunc {
	return func() (string, bool) {
		if len(tryBackends) == 0 {
			return "", false
		}
		minConnections := math.MaxInt32
		var minBackend string
		for _, backend := range tryBackends {
			count := connectionCountManager.GetCount(backend)
			if count < minConnections {
				minConnections = count
				minBackend = backend
			}
		}
		connectionCountManager.Increment(minBackend)
		return minBackend, true
	}
}

var latencyCache = ttlcache.New[string, time.Duration]()

func lowestLatencyNextBackend(tryBackends []string) nextBackendFunc {
	return func() (string, bool) {
		if len(tryBackends) == 0 {
			return "", false
		}
		var lowestBackend string
		var lowestLatency time.Duration
		for _, backend := range tryBackends {
			latencyItem := latencyCache.Get(backend)
			if latencyItem == nil {
				latency := measureLatency(backend)
				latencyCache.Set(backend, latency, time.Minute)
				latencyItem = latencyCache.Get(backend)
			}
			if latencyItem != nil && (lowestLatency == 0 || latencyItem.Value() < lowestLatency) {
				lowestBackend = backend
				lowestLatency = latencyItem.Value()
			}
		}
		return lowestBackend, true
	}
}

func measureLatency(backend string) time.Duration {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", backend, time.Second*5)
	if err != nil {
		return time.Duration(math.MaxInt64) // Return a very high latency if connection fails
	}
	conn.Close()
	return time.Since(start)
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
) (logr.Logger, *packet.StatusResponse, error) {
	log, src, route, nextBackend, err := findRoute(routes, log, client, handshake)
	if err != nil {
		return log, nil, err
	}

	log, res, err := tryBackends(log, nextBackend, func(log logr.Logger, backendAddr string) (logr.Logger, *packet.StatusResponse, error) {
		return resolveStatusResponse(src, dialTimeout, backendAddr, route, log, client, handshake, handshakeCtx, statusRequestCtx)
	})
	if err != nil && route.Fallback != nil {
		log.Info("failed to resolve status response, will use fallback status response", "error", err)

		onlinePlayers := connectionCountManager.GetCount(route.Host[0])
		// Fallback status response if configured
		fallbackPong, err := route.Fallback.Response(handshakeCtx.Protocol, route.MaxConnections, onlinePlayers)
		if err != nil {
			log.Info("failed to get fallback status response", "error", err)
		}
		if fallbackPong != nil {
			status, err2 := json.Marshal(fallbackPong)
			if err2 != nil {
				return log, nil, fmt.Errorf("%w: failed to marshal fallback status response: %w", err, err2)
			}
			if log.V(1).Enabled() {
				log.V(1).Info("using fallback status response", "status", string(status))
			}
			return log, &packet.StatusResponse{Status: string(status)}, nil
		}
	}
	return log, res, err
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
	// Initialize the connection count manager
	connectionCountManager = NewConnectionCountManager()

	go pingCache.Start() // start ttl eviction once

	// Initialize the global and route blacklists
	err := InitBlacklist("./ip_blacklist.json", "./route_blacklist.json")
	if err != nil {
		logger.Error(err, "Failed to initialize blacklists")
	}
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

	pongCtx, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
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

// Initialize logger
func SetLogger(log logr.Logger) {
	logger = log
}


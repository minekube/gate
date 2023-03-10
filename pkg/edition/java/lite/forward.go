package lite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
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
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/netutil"
)

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

	log, src, backendAddr, route, err := findRoute(routes, log, client, handshake)
	if err != nil {
		errs.V(log, err).Info("failed to find route", "error", err)
		return
	}

	dst, err := dialRoute(client.Context(), dialTimeout, src.RemoteAddr(), route, backendAddr, handshake, pc, false)
	if err != nil {
		errs.V(log, err).Info("failed to dial route", "error", err)
		return
	}
	defer func() { _ = dst.Close() }()

	if err = emptyReadBuff(client, dst); err != nil {
		errs.V(log, err).Info("failed to empty client buffer", "error", err)
		return
	}

	log.Info("forwarding connection", "backendAddr", netutil.Host(dst.RemoteAddr()))
	pipe(log, src, dst)
}

func emptyReadBuff(src netmc.MinecraftConn, dst net.Conn) error {
	buff, ok := src.(interface{ ReadBuffered() ([]byte, error) })
	if ok {
		b, err := buff.ReadBuffered()
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

func findRoute(
	routes []config.Route,
	log logr.Logger,
	client netmc.MinecraftConn,
	handshake *packet.Handshake,
) (
	newLog logr.Logger,
	src net.Conn,
	backendAddr string,
	route *config.Route,
	err error,
) {
	srcConn, ok := netmc.Assert[interface{ Conn() net.Conn }](client)
	if !ok {
		return log, src, "", nil, errors.New("failed to assert connection as net.Conn")
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
		return log.V(1), src, "", nil, fmt.Errorf("no route configured for host %s", clearedHost)
	}
	log = log.WithValues("route", host)

	backend := route.Backend.Random()
	if backend == "" {
		return log, src, "", nil, errors.New("no backend configured for route")
	}
	dstAddr, err := netutil.Parse(backend, src.RemoteAddr().Network())
	if err != nil {
		return log, src, "", nil, fmt.Errorf("failed to parse backend address: %w", err)
	}
	backendAddr = dstAddr.String()
	if _, port := netutil.HostPort(dstAddr); port == 0 {
		backendAddr = net.JoinHostPort(dstAddr.String(), "25565")
	}
	log = log.WithValues("backendAddr", backendAddr)

	return log, src, backendAddr, route, nil
}

func dialRoute(
	ctx context.Context,
	dialTimeout time.Duration,
	srcAddr net.Addr,
	route *config.Route,
	routeAddr string,
	handshake *packet.Handshake,
	pc *proto.PacketContext,
	forceUpdatePacketContext bool,
) (dst net.Conn, err error) {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	var dialer net.Dialer
	dst, err = dialer.DialContext(dialCtx, srcAddr.Network(), routeAddr)
	if err != nil {
		v := 0
		if dialCtx.Err() != nil {
			v++
		}
		return nil, &errs.VerbosityError{
			Verbosity: v,
			Err:       fmt.Errorf("failed to connect to backend %s: %w", routeAddr, err),
		}
	}
	defer func() {
		if err != nil {
			_ = dst.Close()
		}
	}()

	if route.ProxyProtocol {
		header := protoutil.ProxyHeader(srcAddr, dst.RemoteAddr())
		if _, err = header.WriteTo(dst); err != nil {
			return dst, fmt.Errorf("failed to write proxy protocol header to backend: %w", err)
		}
	}

	if route.RealIP && IsRealIP(handshake.ServerAddress) {
		// Modify the handshake packet to use RealIP of the client.
		handshake.ServerAddress = RealIP(handshake.ServerAddress, srcAddr)
		forceUpdatePacketContext = true
	}
	if forceUpdatePacketContext {
		update(pc, handshake)
	}

	// Forward handshake packet as is.
	if err = writePacket(dst, pc); err != nil {
		return nil, fmt.Errorf("failed to write handshake packet to backend: %w", err)
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

var pingCache = struct {
	*ttlcache.Cache[string, *pingResult]
	sync.Once
}{}

type pingResult struct {
	res *packet.StatusResponse
	err error
}

func init() {
	pingCache.Cache = ttlcache.New[string, *pingResult]()
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
	log, src, backendAddr, route, err := findRoute(routes, log, client, handshake)
	if err != nil {
		return log, nil, err
	}

	// fast path: use cache
	if route.CachePingEnabled() {
		item := pingCache.Get(backendAddr)
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

		if route.CachePingEnabled() {
			// Always use the latest version when caching is enabled
			handshake.ProtocolVersion = int(version.MaximumVersion.Protocol)
		}

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

	loader := ttlcache.LoaderFunc[string, *pingResult](
		func(c *ttlcache.Cache[string, *pingResult], key string) *ttlcache.Item[string, *pingResult] {
			res, err := load(context.Background())
			pingCache.Do(func() { go pingCache.Start() }) // start ttl eviction once
			return c.Set(backendAddr, &pingResult{res: res, err: err}, route.GetCachePingTTL())
		},
	)

	loaderOpt := ttlcache.WithLoader[string, *pingResult](
		ttlcache.NewSuppressedLoader[string, *pingResult](loader, nil),
	)

	resultChan := make(chan *pingResult, 1)
	go func() { resultChan <- pingCache.Get(backendAddr, loaderOpt).Value() }()

	select {
	case result := <-resultChan:
		return log, result.res, result.err
	case <-client.Context().Done():
		return log, nil, &errs.VerbosityError{
			Err:       client.Context().Err(),
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

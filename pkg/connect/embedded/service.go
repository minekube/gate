package embedded

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/rs/xid"
	"go.minekube.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/netutil"
)

// ServeOptions are Serve options.
type ServeOptions struct { // TODO split tunnel & watch service
	PublicTunnelServiceAddr string         // The tunnel service address announced by every connect.SessionProposal.
	ServerRegistry          ServerRegistry // Registry used to un-/register servers
	Log                     logr.Logger    // Optional logger
}

// ServerRegistry is used to register and retrieve servers.
type ServerRegistry interface {
	// Server gets a registered server by name or returns nil if not found.
	Server(name string) (Server, error)
	// Register registers a server with the proxy.
	Register(Server) error
	// Unregister unregisters the server and returns true if found.
	Unregister(name string) (bool, error)
}

// Server is a server watching for player sessions.
type Server interface {
	Name() string   // The endpoint name self-assigned by the Server.
	connect.Watcher // The watcher representing this Server.
}

// Serve starts serving WatchService and TunnelService until the context is canceled.
// ServeOptions.ServerRegistry is used to register new servers watching for session proposals.
func Serve(ctx context.Context, addr string, opts ServeOptions, grpcServerOpts ...grpc.ServerOption) error {
	// Validation
	if addr == "" {
		return errors.New("missing addr")
	}
	if opts.ServerRegistry == nil {
		return errors.New("missing server registry")
	}
	if opts.PublicTunnelServiceAddr == "" {
		opts.PublicTunnelServiceAddr = addr
	}
	if opts.Log == nil {
		opts.Log = logr.NopLog
	}

	// Listener for Connect services
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	s := services{ServeOptions: opts}
	s.mu.await = map[string]func(connect.InboundTunnel) error{}

	svr := grpc.NewServer(grpcServerOpts...)
	s.register(svr)

	opts.Log.Info("Serving Connect services WatchService & TunnelService")
	go func() { <-ctx.Done(); svr.Stop() }()
	return svr.Serve(ln)
}

type services struct {
	ServeOptions

	mu struct {
		sync.Mutex
		await map[string]func(connect.InboundTunnel) error
	}
}

func (s *services) register(r grpc.ServiceRegistrar) {
	(&connect.TunnelService{AcceptTunnel: s.acceptTunnel}).Register(r)
	(&connect.WatchService{StartWatch: s.startWatch}).Register(r)
}

func (s *services) startWatch(watcher connect.Watcher) error {
	endpoint := valueFrom(watcher.Context(), connect.MDEndpoint, metadata.FromIncomingContext)
	if endpoint == "" {
		return status.Error(codes.InvalidArgument, "missing endpoint in request metadata")
	}
	svr := &server{
		s:       s,
		log:     s.Log.WithName("endpoint").WithName(endpoint),
		Watcher: watcher,
		name:    endpoint,
	}
	svr.mu.rejectedFns = map[string]func(rejection *connect.SessionRejection){}
	if err := s.ServerRegistry.Register(svr); err != nil {
		return err
	}
	defer func() {
		_, err := s.ServerRegistry.Unregister(svr.Name())
		if err != nil {
			s.Log.Error(err, "Error unregistering server", "name", svr.Name())
		}
	}()
	go svr.startRejectionMultiplexer()
	<-watcher.Context().Done()
	return nil
}

func (s *services) acceptTunnel(tunnel connect.InboundTunnel) error {
	sessionID := valueFrom(tunnel.Context(), connect.MDSession, metadata.FromIncomingContext)
	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "missing session id in request metadata")
	}
	s.mu.Lock()
	fn, ok := s.mu.await[sessionID]
	if ok {
		delete(s.mu.await, sessionID)
	}
	s.mu.Unlock()
	if !ok {
		return status.Error(codes.NotFound, "could not found session id, the session proposal might be canceled already")
	}
	return fn(tunnel)
}

func (s *services) awaitTunnel(ctx context.Context, sessionID string) (tunnelChan <-chan connect.InboundTunnel, remove func()) {
	ch := make(chan connect.InboundTunnel)
	returnFn := func(tunnel connect.InboundTunnel) error {
		select {
		case ch <- tunnel:
			return nil
		case <-ctx.Done():
			return status.Error(codes.Canceled, "session proposal was canceled")
		}
	}
	s.mu.Lock()
	s.mu.await[sessionID] = returnFn
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.mu.await, sessionID)
		s.mu.Unlock()
	}
}

type server struct {
	s   *services
	log logr.Logger
	connect.Watcher
	name string
	mu   struct {
		sync.Mutex
		rejectedFns map[string]func(rejection *connect.SessionRejection)
	}
}

func (s *server) Name() string { return s.name }

func (*server) HandshakeAddr(defaultAddr string, _ proxy.Player) string {
	// For tunnel servers we don't modify the ServerAddress in the Handshake packet,
	// no matter if the target server (e.g. spigot) is in bungee/velocity forwarding mode,
	// the java Connect plugin takes care of injecting the correct player data from the session proposal
	return defaultAddr
}

var _ proxy.HandshakeAddresser = (*server)(nil)

func init() { rand.Seed(time.Now().UnixNano()) }

var _ proxy.ServerDialer = (*server)(nil)

// Dial proposes a session to the watching target server, waits for the target server
// to create a TunnelConn with the TunnelService listening at ServeOptions.PublicTunnelServiceAddr
// and returns the TunnelConn.
//
// It is recommended to always pass a timeout context because the target server might never
// create the TunnelConn with the TunnelService.
//
// Dial unblocks on the following events:
//  - If the target server has established a TunnelConn successfully.
//  - If the passed context is canceled, cleans up and cancels the session proposal.
//  - If the server rejected the session proposal wrapping the given status reason in the returned error if present.
//  - If the server's watcher has disconnected.
//
func (s *server) Dial(ctx context.Context, p proxy.Player) (net.Conn, error) {
	session := &connect.Session{
		Id:                xid.New().String(),
		TunnelServiceAddr: s.s.PublicTunnelServiceAddr,
		Player:            newConnectPlayer(p),
	}

	log := s.log.
		WithValues("username", p.Username()).
		WithValues("sessionID", session.GetId())
	log.Info("Proposing session for player")

	// Using a less timely context timeout if the parent ctx never cancels.
	ctx, cancel := context.WithTimeout(ctx, time.Hour/2)
	defer cancel()

	tunnelChan, removeAwaitTunnel := s.s.awaitTunnel(ctx, session.GetId())
	defer removeAwaitTunnel()

	rejectionChan, removeListener := s.listenForRejection(ctx, session.GetId())
	defer removeListener()

	// Propose session to watcher
	if err := s.Watcher.Propose(session); err != nil {
		return nil, fmt.Errorf("could not propose player session to target server: %w", err)
	}
	// Wait for response or cancellation
	select {
	case tunnel := <-tunnelChan:
		s.log.Info("Prepared session for player")
		return tunnel.Conn(), nil
	case r := <-rejectionChan:
		s.log.Info("Session proposal rejected by server", "reason", r.GetReason())
		if r.GetReason() != nil {
			return nil, fmt.Errorf("session proposal rejected by server: %w", status.FromProto(r.GetReason()).Err())
		}
		return nil, errors.New("session proposal rejected by server without reason")
	case <-s.Watcher.Context().Done():
		return nil, fmt.Errorf("server has disconnected: %w", s.Watcher.Context().Err())
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *server) listenForRejection(ctx context.Context, sessionID string) (rejectionChan <-chan *connect.SessionRejection, remove func()) {
	ch := make(chan *connect.SessionRejection)
	returnFn := func(rejection *connect.SessionRejection) {
		select {
		case ch <- rejection:
		case <-ctx.Done():
		}
	}
	s.mu.Lock()
	s.mu.rejectedFns[sessionID] = returnFn
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.mu.rejectedFns, sessionID)
		s.mu.Unlock()
	}
}

func (s *server) startRejectionMultiplexer() {
	for {
		rejection, ok := <-s.Watcher.Rejections()
		if !ok {
			return
		}
		s.mu.Lock()
		fn, ok := s.mu.rejectedFns[rejection.GetId()]
		if ok {
			delete(s.mu.rejectedFns, rejection.GetId())
		}
		s.mu.Unlock()
		if !ok {
			s.log.V(1).Info("Received unexpected session rejection",
				"sessionID", rejection.GetId(),
				"reason", rejection.GetReason())
			continue
		}
		fn(rejection)
	}
}

func newConnectPlayer(p proxy.Player) *connect.Player {
	prof := p.GameProfile()
	props := make([]*connect.GameProfileProperty, len(prof.Properties))
	for i, prop := range prof.Properties {
		props[i] = &connect.GameProfileProperty{
			Name:      prop.Name,
			Value:     prop.Value,
			Signature: prop.Signature,
		}
	}
	return &connect.Player{
		Addr: netutil.Host(p.RemoteAddr()),
		Profile: &connect.GameProfile{
			Id:         prof.ID.String(),
			Name:       prof.Name,
			Properties: props,
		},
	}
}

func valueFrom(ctx context.Context, key string, mdFn func(ctx context.Context) (metadata.MD, bool)) string {
	md, _ := mdFn(ctx)
	if s := md.Get(key); len(s) != 0 {
		return s[0]
	}
	return ""
}

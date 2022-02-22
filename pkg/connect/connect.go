package connect

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/rs/xid"
	"go.minekube.com/connect"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Options are Start options.
type Options struct {
	PublicTunnelServiceAddr string // The tunnel service address announced by every connect.SessionProposal.
	Log                     logr.Logger
	ServerRegistry          ServerRegistry
}

// ServerRegistry is used to register and retrieve servers.
type ServerRegistry interface {
	// Server gets a registered server by name or returns nil if not found.
	Server(name string) Server
	// Register registers a server with the proxy.
	Register(Server) error
	// Unregister unregisters the server and returns true if found.
	Unregister(name string) bool
}

type Server interface {
	Name() string   // The endpoint name self-assigned by the Server.
	connect.Watcher // The watcher representing this Server.
}

// Start starts Connect services WatchService and TunnelService until the context is canceled.
func Start(ctx context.Context, addr string, opts Options) error {
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

	s := services{Options: opts}
	s.mu.await = map[string]func(connect.InboundTunnel) error{}

	svr := grpc.NewServer()
	s.register(svr)

	opts.Log.Info("Serving Connect services WatchService & TunnelService")
	go func() { <-ctx.Done(); svr.Stop() }()
	return svr.Serve(ln)
}

type services struct {
	Options

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
	endpoint := valueFromContext(watcher.Context(), connect.MDEndpoint, true)
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
	go svr.startRejectionMultiplexer()
	<-watcher.Context().Done()
	_ = s.ServerRegistry.Unregister(svr.Name())
	return nil
}

func (s *services) acceptTunnel(tunnel connect.InboundTunnel) error {
	sessionID := valueFromContext(tunnel.Context(), connect.MDSession, true)
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
		return status.Error(codes.NotFound, "could not found session id")
	}

	return fn(tunnel)
}

func (s *services) awaitTunnel(ctx context.Context, sessionID string) (tunnelChan <-chan connect.InboundTunnel, remove func()) {
	ch := make(chan connect.InboundTunnel)
	s.mu.Lock()
	s.mu.await[sessionID] = func(tunnel connect.InboundTunnel) error {
		select {
		case ch <- tunnel:
			return nil
		case <-ctx.Done():
			return status.Error(codes.Canceled, "session proposal was canceled")
		}
	}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.mu.await, sessionID)
		s.mu.Unlock()
	}
}

// todo move somewhere appropriate
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

// Dial returns a TunnelConn to the server accessible through Connect.
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

	ctx, cancel := context.WithCancel(ctx)
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
		return nil, fmt.Errorf("session proposal rejected by server: %w", status.FromProto(r.GetReason()).Err())
	case <-s.Watcher.Context().Done():
		return nil, fmt.Errorf("server has disconnected: %w", s.Watcher.Context().Err())
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *server) listenForRejection(ctx context.Context, sessionID string) (rejectionChan <-chan *connect.SessionRejection, remove func()) {
	ch := make(chan *connect.SessionRejection)
	s.mu.Lock()
	s.mu.rejectedFns[sessionID] = func(rejection *connect.SessionRejection) {
		select {
		case ch <- rejection:
		case <-ctx.Done():
		}
	}
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

// func watch() {
// p.log.Info("Dialing watch...")
// conn, err := grpc.DialContext(ctx, ":8443", grpc.WithInsecure(), grpc.WithBlock())
// if err != nil {
// 	return err
// }
// defer conn.Close()
// p.log.Info("Watching...")
// ctx = metadata.AppendToOutgoingContext(ctx, connect.MDEndpoint, "server1")
// return connect.Watch(ctx, connect.WatchOptions{
// 	Cli: connect.NewWatchServiceClient(conn),
// 	Callback: func(proposal connect.SessionProposal) (err error) {
// 		defer func() {
// 			if err != nil {
// 				_ = proposal.Reject(status.FromContextError(err).Proto())
// 			}
// 		}()
// 		p.log.Info("Establishing tunnel for new session")
// 		var tunnelCli *grpc.ClientConn
// 		tunnelCli, err = grpc.DialContext(ctx, proposal.Session().GetTunnelServiceAddr(), grpc.WithInsecure(), grpc.WithBlock())
// 		if err != nil {
// 			return err
// 		}
// 		ctx := metadata.AppendToOutgoingContext(ctx, connect.MDSession, proposal.Session().GetId())
// 		fmt.Println("sessionID:", valueFromContext(ctx, connect.MDSession, false))
// 		tc, err := connect.Tunnel(ctx, connect.TunnelOptions{
// 			TunnelCli:  connect.NewTunnelServiceClient(tunnelCli),
// 			LocalAddr:  "server1:1234",
// 			RemoteAddr: proposal.Session().GetPlayer().GetAddr(),
// 		})
// 		if err != nil {
// 			return err
// 		}
// 		p.log.Info("Established tunnel for new session")
// 		go p.handleRawConn(&tunnelConn{
// 			TunnelConn: tc,
// 			s:          proposal.Session(),
// 		})
// 		return nil
// 	},
// })
// }

type TunnelConn interface {
	connect.TunnelConn
	Session() *connect.Session
}

type tunnelConn struct {
	connect.TunnelConn
	s  *connect.Session
	gp *profile.GameProfile

	debugRead  func(b []byte) (int, error)
	debugWrite func(b []byte) (int, error)
	countRead  atomic.Uint64
	countWrite atomic.Uint64
	prefRead   string
	prefWrite  atomic.String
}

func (t *tunnelConn) GameProfile() *profile.GameProfile { return t.gp }

var _ proxy.GameProfileProvider = (*tunnelConn)(nil)

func (t *tunnelConn) Session() *connect.Session { return t.s }
func (t *tunnelConn) Read(b []byte) (n int, err error) {
	defer func() {
		s := humanize.Bytes(t.countRead.Add(uint64(n)))
		if t.prefRead != s {
			t.prefRead = s
			fmt.Println("read", s)
		}
	}()
	return t.TunnelConn.Read(b)
}
func (t *tunnelConn) Write(b []byte) (n int, err error) {
	defer func() {
		s := humanize.Bytes(t.countWrite.Add(uint64(n)))
		if t.prefWrite.Load() != s {
			t.prefWrite.Store(s)
			fmt.Println("write", s)
		}
	}()
	return t.TunnelConn.Write(b)
}

var _ TunnelConn = (*tunnelConn)(nil)

func gameProfileFromSessionGameProfile(p *connect.GameProfile) (*profile.GameProfile, error) {
	id, err := uuid.Parse(p.GetId())
	if err != nil {
		return nil, fmt.Errorf("invalid player id: %w", err)
	}
	props := make([]profile.Property, len(p.Properties))
	for i, prop := range p.Properties {
		props[i] = profile.Property{
			Name:      prop.GetName(),
			Value:     prop.GetValue(),
			Signature: prop.GetSignature(),
		}
	}
	return &profile.GameProfile{
		ID:         id,
		Name:       p.GetName(),
		Properties: props,
	}, nil
}

func valueFromContext(ctx context.Context, key string, incoming bool) string {
	var md metadata.MD
	if incoming {
		md, _ = metadata.FromIncomingContext(ctx)
	} else {
		md, _ = metadata.FromOutgoingContext(ctx)
	}
	if s := md.Get(key); len(s) != 0 {
		return s[0]
	}
	return ""
}

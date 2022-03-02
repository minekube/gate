// Package single combines connect.EndpointAcceptor and connect.TunnelAcceptor
// into Acceptor allowing to run WatchService and TunnelService in the same instance.
package single

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/xid"
	connct "go.minekube.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/runtime/logr"
	connect2 "go.minekube.com/gate/pkg/util/connectutil"
	"go.minekube.com/gate/pkg/util/netutil"
)

type Options struct {
	Log                     logr.Logger          // Optional logger
	ServerRegistry          proxy.ServerRegistry // Registry used to un-/register servers
	PublicTunnelServiceAddr string               // The tunnel service address announced endpoints.
	OverrideRegistration    bool                 // Overrides endpoints with the same name.
}

type Acceptor interface {
	connect2.EndpointAcceptor
	connect2.TunnelAcceptor
}

func New(opts Options) (Acceptor, error) {
	if opts.ServerRegistry == nil {
		return nil, errors.New("missing server registry")
	}
	if opts.PublicTunnelServiceAddr == "" {
		return nil, errors.New("missing server public tunnel service address")
	}
	if opts.Log == nil {
		opts.Log = logr.NopLog
	}
	return &acceptor{
		Options:         opts,
		pendingSessions: sessionTunnel{},
	}, nil
}

type sessionTunnel map[string]func(connect2.InboundTunnel) error

type acceptor struct {
	Options
	mu              sync.Mutex    // protects following
	pendingSessions sessionTunnel // sessions waiting for inbound tunnel
}

func (a *acceptor) AcceptEndpoint(endpoint connect2.Endpoint) error {
	if a.OverrideRegistration {
		if rs := a.ServerRegistry.Server(endpoint.Name()); rs != nil {
			if s, ok := rs.ServerInfo().(*server); ok {
				// Stop servers watcher first
				s.disconnect(status.Error(codes.Canceled, "another endpoint with the same name has registered"))
			} else {
				a.ServerRegistry.Unregister(rs.ServerInfo())
			}
		}
	}

	// Prepare endpoint for registration as server
	svr := &server{
		a:               a,
		Endpoint:        endpoint,
		log:             a.Log.WithName("endpoint").WithName(endpoint.Name()),
		addr:            netutil.NewAddr(endpoint.Name()+":25565", "connect"),
		pendingSessions: rejectSession{},
	}

	// Allows OverrideRegistration disconnect server with same name
	disconnect := make(chan error)
	var once sync.Once
	svr.disconnect = func(err error) {
		once.Do(func() {
			_ = a.ServerRegistry.Unregister(svr)
			select {
			case disconnect <- err:
			case <-endpoint.Context().Done():
			}
		})
	}

	// Try register server
	if _, err := a.ServerRegistry.Register(svr); err != nil {
		if errors.Is(err, proxy.ErrServerAlreadyExists) {
			return status.Error(codes.AlreadyExists, "another endpoint with the same name is already registered")
		}
		return status.Errorf(codes.InvalidArgument, "invalid endpoint definition: %v", err)
	}

	go func() { <-endpoint.Context().Done(); svr.disconnect(nil) }()
	go svr.startRejectionMultiplexer()
	return <-disconnect
}

func (a *acceptor) AcceptTunnel(tunnel connect2.InboundTunnel) error {
	a.mu.Lock()
	accept, ok := a.pendingSessions[tunnel.SessionID()]
	if ok {
		delete(a.pendingSessions, tunnel.SessionID())
	}
	a.mu.Unlock()
	if !ok {
		return status.Error(codes.NotFound, "could not found session id, the session proposal might be canceled already")
	}
	return accept(tunnel)
}

type rejectSession map[string]func(rejection *connct.SessionRejection)

type server struct {
	a *acceptor
	connect2.Endpoint
	addr            net.Addr
	disconnect      func(err error)
	log             logr.Logger
	mu              sync.Mutex // protects following
	pendingSessions rejectSession
}

var _ proxy.ServerInfo = (*server)(nil)

func (s *server) Addr() net.Addr { return s.addr }

func (s *server) addPendingSession(ctx context.Context, sessionID string) (
	<-chan connct.InboundTunnel, <-chan *connct.SessionRejection, context.CancelFunc) {

	tunnelCh := make(chan connct.InboundTunnel)
	rejectCh := make(chan *connct.SessionRejection)

	tunnel := func(tunnel connect2.InboundTunnel) error {
		select {
		case tunnelCh <- tunnel:
			return nil
		case <-ctx.Done():
			return status.Error(codes.Canceled, "session proposal was canceled")
		}
	}
	reject := func(rejection *connct.SessionRejection) {
		select {
		case rejectCh <- rejection:
		case <-ctx.Done():
		}
	}

	// Add pending session
	s.mu.Lock()
	s.pendingSessions[sessionID] = reject
	s.mu.Unlock()

	s.a.mu.Lock()
	s.a.pendingSessions[sessionID] = tunnel
	s.a.mu.Unlock()

	remove := func() {
		// Remove session if still pending
		s.mu.Lock()
		delete(s.pendingSessions, sessionID)
		s.mu.Unlock()

		s.a.mu.Lock()
		delete(s.a.pendingSessions, sessionID)
		s.a.mu.Unlock()
	}

	return tunnelCh, rejectCh, remove
}

// implementing Dial allows Gate to create a tunnel to a server for a player
var _ proxy.ServerDialer = (*server)(nil)

// Dial establishes a Tunnel with an Endpoint.
//
// It proposes a session to the endpoint, waits for the endpoint to create a
// Tunnel with the TunnelService listening at PublicTunnelServiceAddr
// and returns the Tunnel.
//
// It is recommended to always pass a timeout context because the endpoint might never
// create the Tunnel with the TunnelService.
//
// Dial unblocks on the following events:
//  - If the endpoint has established a Tunnel successfully.
//  - If the passed context is canceled, cleans up and cancels the session proposal.
//  - If the endpoint rejected the session proposal wrapping the given status reason in the returned error if present.
//  - If the endpoint's watcher has disconnected / was unregistered.
//
func (s *server) Dial(ctx context.Context, p proxy.Player) (net.Conn, error) {
	session := &connct.Session{
		Id:                xid.New().String(),
		TunnelServiceAddr: s.a.PublicTunnelServiceAddr,
		Player:            newConnectPlayer(p),
	}

	log := s.log.
		WithValues("username", p.Username()).
		WithValues("sessionID", session.GetId())
	log.Info("Proposing session for player")

	// Using a less timely context timeout if the parent ctx never cancels.
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	tunnelChan, rejectionChan, remove := s.addPendingSession(ctx, session.GetId())
	defer remove()

	// Propose session to endpoint
	if err := s.Propose(session); err != nil {
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
	case <-s.Context().Done():
		return nil, fmt.Errorf("server has disconnected: %w", s.Context().Err())
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// implementing this leaves the handshake address as is for tunneled player connections
var _ proxy.HandshakeAddresser = (*server)(nil)

func (*server) HandshakeAddr(defaultAddr string, _ proxy.Player) string {
	// For tunnel servers we don't modify the ServerAddress in the Handshake packet,
	// no matter if the target server (e.g. spigot) is in bungee/velocity forwarding mode,
	// the java Connect plugin takes care of injecting the correct player data from the session proposal
	return defaultAddr
}

func (s *server) startRejectionMultiplexer() {
	for {
		rejection, ok := <-s.Rejections()
		if !ok {
			return
		}
		s.mu.Lock()
		fn, ok := s.pendingSessions[rejection.GetId()]
		if ok {
			delete(s.pendingSessions, rejection.GetId())
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

func newConnectPlayer(p proxy.Player) *connct.Player {
	prof := p.GameProfile()
	props := make([]*connct.GameProfileProperty, len(prof.Properties))
	for i, prop := range prof.Properties {
		props[i] = &connct.GameProfileProperty{
			Name:      prop.Name,
			Value:     prop.Value,
			Signature: prop.Signature,
		}
	}
	return &connct.Player{
		Addr: netutil.Host(p.RemoteAddr()),
		Profile: &connct.GameProfile{
			Id:         prof.ID.String(),
			Name:       prof.Name,
			Properties: props,
		},
	}
}

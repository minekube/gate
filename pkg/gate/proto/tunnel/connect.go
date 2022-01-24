package tunnel

import (
	"context"
	"errors"
	"fmt"
	"github.com/rs/xid"
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/util/validation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"net"
	"sync"
)

// EndpointAddedEvent is fired when an endpoint was added.
// Note that multiple endpoints can have the same name.
type EndpointAddedEvent struct {
	Endpoint pb.Endpoint
}

// EndpointRemovedEvent is fired when an endpoint was removed.
// Note that multiple endpoints can have the same name.
type EndpointRemovedEvent struct {
	Endpoint pb.Endpoint
}

type Connect struct {
	// Optional event emitter that fires
	// EndpointAddedEvent and EndpointRemovedEvent.
	Event event.Manager

	svc *service
}

func (c *Connect) ListenAndServer(ctx context.Context, localAddr, publicTunnelSvcAddr string) error {
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("error listening on %q: %w", localAddr, err)
	}
	defer func() { _ = ln.Close() }()

	if c.Event == nil {
		c.Event = event.Nop
	}
	c.svc = &service{
		c:             c,
		tunnelSvcAddr: publicTunnelSvcAddr,
		localAddr:     ln.Addr(),
	}
	c.svc.awaitingTunnel.sessions = map[string]*dialTunnelRequest{}
	defer func() { c.svc = nil }()

	svr := grpc.NewServer()
	pb.RegisterConnectServiceServer(svr, c.svc)
	pb.RegisterTunnelServiceServer(svr, c.svc)

	// Stop serving on canceled context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { <-ctx.Done(); svr.Stop() }()

	return svr.Serve(ln)
}

// Dial establishes a tunnel connection with the endpoint for a player.
func (c *Connect) Dial(ctx context.Context, endpoint string, player *pb.Player) (net.Conn, error) {
	// Validation
	if c.svc == nil {
		return nil, errors.New("ListenAndServer must be called before Dial")
	}
	if player == nil {
		return nil, errors.New("player must not be nil")
	}
	if player.GetProfile().GetId() == "" {
		return nil, errors.New("missing player profile id")
	}
	if player.GetProfile().GetName() == "" {
		return nil, errors.New("missing player profile name")
	}

	// Get a watcher for that endpoint
	w := c.svc.watchers.random(endpoint)
	if w == nil {
		return nil, fmt.Errorf("no active tunnel for endpoint %q", endpoint)
	}

	sessionID := xid.New().String()

	// await tunnel connection
	responseChan := make(chan *dialTunnelRequest, 1)
	c.svc.awaitingTunnel.Lock()
	c.svc.awaitingTunnel.sessions[sessionID] = &dialTunnelRequest{
		dialCtx:      ctx,
		responseChan: responseChan,
	}
	c.svc.awaitingTunnel.Unlock()

	defer func() {
		// Insure session is removed if not already done by request to Tunnel rpc
		c.svc.awaitingTunnel.Lock()
		delete(c.svc.awaitingTunnel.sessions, sessionID)
		c.svc.awaitingTunnel.Unlock()
	}()

	// Inform the watcher that it should connect to the TunnelService
	// to establish an outbound connection to start the session
	err := w.stream.Send(&pb.WatchResponse{Message: &pb.WatchResponse_StartSession{
		StartSession: &pb.StartSession{
			Id:                sessionID,
			TunnelServiceAddr: c.svc.tunnelSvcAddr,
			Player:            player,
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("could not send StartSession to an endpoint: %w", err)
	}

	select {
	case result := <-responseChan:
		return result.conn, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type service struct {
	c             *Connect
	tunnelSvcAddr string
	localAddr     net.Addr // listener

	watchers watchers

	awaitingTunnel struct {
		sync.RWMutex
		sessions map[string]*dialTunnelRequest
	}

	pb.UnimplementedConnectServiceServer
	pb.UnimplementedTunnelServiceServer
}

func (s *service) Watch(req *pb.WatchRequest, stream pb.ConnectService_WatchServer) error {
	// Validate request
	endpoint := req.GetEndpoint().GetName()
	if endpoint == "" {
		return status.Error(codes.InvalidArgument, "missing endpoint name")
	}
	if !validation.ValidServerName(endpoint) {
		return status.Error(codes.InvalidArgument, "unqualified endpoint name")
	}

	// Add endpoint
	w := s.watchers.add(req.GetEndpoint(), stream)
	defer func() {
		// Remove watching endpoint.
		// There might be active tunnel connections for that endpoint,
		// which are decoupled from this Watch rpc.
		s.watchers.remove(endpoint, w)
		s.c.Event.Fire(&EndpointRemovedEvent{Endpoint: *req.GetEndpoint()})
	}()
	s.c.Event.Fire(&EndpointAddedEvent{Endpoint: *req.GetEndpoint()})

	// Block until stream is closed
	<-stream.Context().Done()
	return nil
}

type dialTunnelRequest struct {
	conn    net.Conn
	dialCtx context.Context

	responseChan chan<- *dialTunnelRequest
}

func (s *service) Tunnel(biStream pb.TunnelService_TunnelServer) error {
	// first message must receive session id
	req, err := biStream.Recv() // TODO add timout
	if err != nil {
		return err
	}
	sessionID := req.GetSessionId()
	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "first message must be the session id")
	}

	// Get awaiting tunnel (dialer) for session id
	s.awaitingTunnel.Lock()
	awaiting, ok := s.awaitingTunnel.sessions[sessionID]
	if ok {
		delete(s.awaitingTunnel.sessions, sessionID)
	}
	s.awaitingTunnel.Unlock()
	if !ok {
		return status.Errorf(codes.NotFound,
			"session %q not found or already established by another watcher",
			sessionID)
	}

	closeTunnelErr := make(chan error)
	var closeOnce sync.Once
	closeTunnel := func(err error) { closeOnce.Do(func() { closeTunnelErr <- err }) }

	go func() {
		// Setup connection
		p, _ := peer.FromContext(biStream.Context())
		awaiting.conn = newConn(sessionID, s.localAddr, p.Addr, biStream, closeTunnel)
		// Send back to watcher
		awaiting.responseChan <- awaiting
	}()

	return <-closeTunnelErr
}

type Dialer interface {
	Dial(ctx context.Context, intent *pb.StartSession) (net.Conn, error)
}

type watchers struct {
	sync.RWMutex
	// key = endpoint name
	m map[string]watcherSet
}
type watcherSet map[*watcher]struct{}

type watcher struct {
	*pb.Endpoint
	stream pb.ConnectService_WatchServer
}

func (m *watchers) add(e *pb.Endpoint, stream pb.ConnectService_WatchServer) *watcher {
	w := &watcher{
		Endpoint: e,
		stream:   stream,
	}
	m.Lock()
	set, ok := m.m[e.GetName()]
	if !ok {
		set = make(watcherSet, 1)
		if m.m == nil {
			m.m = map[string]watcherSet{}
		}
		m.m[e.GetName()] = set
	}
	set[w] = struct{}{}
	m.Unlock()
	return w
}

func (m *watchers) remove(endpoint string, w *watcher) {
	m.Lock()
	set := m.m[endpoint]
	delete(set, w)
	if len(set) == 0 {
		delete(m.m, endpoint)
	}
	m.Unlock()
}

// returns nil if not found
func (m *watchers) random(endpoint string) *watcher {
	m.RLock()
	set := m.m[endpoint]
	m.RUnlock()
	// relying on random map iteration
	for w := range set {
		return w
	}
	return nil
}

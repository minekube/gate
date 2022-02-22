package proxy

import (
	"context"
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/rs/xid"
	"go.minekube.com/connect"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

// todo move somewhere appropriate
type tunnelServerInfo struct {
	log logr.Logger
	ServerInfo
	connect.Watcher

	mu struct {
		sync.RWMutex
		await map[string]chan<- connect.InboundTunnel
	}
}

func init() { rand.Seed(time.Now().UnixNano()) }

func (t *tunnelServerInfo) Dial(ctx context.Context, p Player) (connect.TunnelConn, error) {
	t.log.Info("Creating tunnel for player")
	session := &connect.Session{
		Id:                xid.New().String(),
		TunnelServiceAddr: "localhost:8443",
		Player:            newConnectPlayer(p),
	}
	fmt.Println("created sessionid", session.Id)

	tunnelChan := make(chan connect.InboundTunnel)
	t.mu.Lock()
	if t.mu.await == nil {
		t.mu.await = map[string]chan<- connect.InboundTunnel{}
	}
	t.mu.await[session.GetId()] = tunnelChan
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.mu.await, session.GetId())
		t.mu.Unlock()
	}()

	// Propose session to watcher
	err := t.Watcher.Propose(session)
	if err != nil {
		return nil, fmt.Errorf("could not propose player session to target server: %w", err)
	}
	// Wait for inbound tunnel
	select {
	case r, ok := <-t.Watcher.Rejections():
		if !ok {
			return nil, fmt.Errorf("watcher stopped: %w", t.Watcher.Context().Err())
		}
		t.log.Info("Session rejected", "sessionID", r.GetId(), "reason", r.GetReason())
		return nil, status.FromProto(r.GetReason()).Err()
	case tunnel := <-tunnelChan:
		t.log.Info("Created tunnel for player")
		return tunnel.Conn(), nil
	case <-ctx.Done():
		t.log.Info("Creating tunnel context canceled")
		return nil, ctx.Err()
	}
}

func newConnectPlayer(p Player) *connect.Player {
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

func (p *Proxy) watchConnect(ctx context.Context) error {
	if os.Getenv("connect") == "true" {
		ln, err := net.Listen("tcp", ":8443")
		if err != nil {
			return err
		}
		defer ln.Close()
		ts := &connect.TunnelService{
			AcceptTunnel: func(tunnel connect.InboundTunnel) error {
				sessionID := valueFromContext(tunnel.Context(), connect.MDSession, true)
				if sessionID == "" {
					return status.Error(codes.InvalidArgument, "missing session id in request metadata")
				}

				for _, s := range p.Servers() {
					t, ok := s.ServerInfo().(*tunnelServerInfo)
					if !ok {
						continue
					}
					t.mu.RLock()
					ch, ok := t.mu.await[sessionID]
					t.mu.RUnlock()
					if ok {
						p.log.Info("Accepted new tunnel")
						ch <- tunnel
						return nil
					}
				}
				return status.Error(codes.NotFound, "could not found session id")
			},
		}
		ws := &connect.WatchService{
			StartWatch: func(watcher connect.Watcher) error {
				endpoint := valueFromContext(watcher.Context(), connect.MDEndpoint, true)
				if endpoint == "" {
					return status.Error(codes.InvalidArgument, "missing endpoint in request metadata")
				}
				info := NewServerInfo(
					endpoint,
					// todo don't need port
					netutil.NewAddr("tcp", endpoint, 0),
				)
				tsi := &tunnelServerInfo{
					log:        p.log.WithName(info.Name()),
					Watcher:    watcher,
					ServerInfo: info,
				}
				if existing := p.Server(info.Name()); existing != nil {
					p.Unregister(existing.ServerInfo())
				}
				p.Register(tsi)
				<-watcher.Context().Done()
				fmt.Println(watcher.Context().Err())
				p.Unregister(info)
				return nil
			},
		}
		svr := grpc.NewServer()
		ts.Register(svr)
		ws.Register(svr)
		p.log.Info("Serving Connect services...")
		go func() { <-ctx.Done(); svr.Stop() }()
		return svr.Serve(ln)
	}

	p.log.Info("Dialing watch...")
	conn, err := grpc.DialContext(ctx, ":8443", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	p.log.Info("Watching...")
	ctx = metadata.AppendToOutgoingContext(ctx, connect.MDEndpoint, "server1")
	return connect.Watch(ctx, connect.WatchOptions{
		Cli: connect.NewWatchServiceClient(conn),
		Callback: func(proposal connect.SessionProposal) (err error) {
			defer func() {
				if err != nil {
					_ = proposal.Reject(status.FromContextError(err).Proto())
				}
			}()
			p.log.Info("Establishing tunnel for new session")
			var tunnelCli *grpc.ClientConn
			tunnelCli, err = grpc.DialContext(ctx, proposal.Session().GetTunnelServiceAddr(), grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				return err
			}
			ctx := metadata.AppendToOutgoingContext(ctx, connect.MDSession, proposal.Session().GetId())
			fmt.Println("sessionID:", valueFromContext(ctx, connect.MDSession, false))
			tc, err := connect.Tunnel(ctx, connect.TunnelOptions{
				TunnelCli:  connect.NewTunnelServiceClient(tunnelCli),
				LocalAddr:  "server1:1234",
				RemoteAddr: proposal.Session().GetPlayer().GetAddr(),
			})
			if err != nil {
				return err
			}
			p.log.Info("Established tunnel for new session")
			go p.handleRawConn(&tunnelConn{
				TunnelConn: tc,
				s:          proposal.Session(),
			})
			return nil
		},
	})
}

type TunnelConn interface {
	connect.TunnelConn
	Session() *connect.Session
}

type tunnelConn struct {
	connect.TunnelConn
	s *connect.Session

	debugRead  func(b []byte) (int, error)
	debugWrite func(b []byte) (int, error)
	countRead  atomic.Uint64
	countWrite atomic.Uint64
	prefRead   string
	prefWrite  atomic.String
}

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

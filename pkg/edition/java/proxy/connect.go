package proxy

import (
	"context"
	"fmt"
	"github.com/rs/xid"
	"go.minekube.com/connect"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"os"
	"sync"
)

// todo move somewhere appropriate
type tunnelServerInfo struct {
	ServerInfo
	connect.Watcher

	mu struct {
		sync.RWMutex
		await map[string]chan<- connect.InboundTunnel
	}
}

func (t *tunnelServerInfo) Dial(ctx context.Context, p Player) (connect.TunnelConn, error) {
	session := &connect.Session{
		Id:                xid.New().String(),
		TunnelServiceAddr: ":8443",
		Player:            newConnectPlayer(p),
	}

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
	case tunnel := <-tunnelChan:
		return tunnel.Conn(), nil
	case <-ctx.Done():
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
	return &connect.Player{Profile: &connect.GameProfile{
		Id:         prof.ID.String(),
		Name:       prof.Name,
		Properties: props,
	}}
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
				for _, s := range p.Servers() {
					t, ok := s.ServerInfo().(*tunnelServerInfo)
					if !ok {
						continue
					}
					t.mu.RLock()
					ch, ok := t.mu.await[tunnel.SessionID()]
					t.mu.RUnlock()
					if ok {
						ch <- tunnel
						return nil
					}
				}
				return status.Error(codes.NotFound, "could not found session id")
			},
		}
		ws := &connect.WatchService{
			StartWatch: func(watcher connect.Watcher) error {
				info := NewServerInfo(
					watcher.Endpoint().GetName(),
					// todo don't need port
					netutil.NewAddr("tcp", watcher.Endpoint().GetName(), 0),
				)
				tsi := &tunnelServerInfo{
					Watcher:    watcher,
					ServerInfo: info,
				}
				p.Register(tsi)
				<-watcher.Context().Done() // todo WHY IS THIS CONTEXT CANCELED?????
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

	conn, err := grpc.DialContext(ctx, ":8443", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return err
	}
	//defer conn.Close()
	p.log.Info("Watching...")
	return connect.Watch(ctx, connect.WatchOptions{
		Cli:      connect.NewWatchServiceClient(conn),
		Endpoint: "endpoint1",
		Callback: func(proposal connect.SessionProposal) error {
			p.log.Info("Establishing tunnel for new session")
			tunnelCli, err := grpc.DialContext(ctx, proposal.Session().GetTunnelServiceAddr(), grpc.WithInsecure())
			if err != nil {
				return err
			}
			tc, err := connect.Tunnel(ctx, connect.TunnelOptions{
				TunnelCli:  connect.NewTunnelServiceClient(tunnelCli),
				SessionID:  proposal.Session().GetId(),
				LocalAddr:  "endpoint1",
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
}

func (t *tunnelConn) Session() *connect.Session { return t.s }

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

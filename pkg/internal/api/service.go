package api

import (
	"context"
	"errors"
	"fmt"
	"net"

	"connectrpc.com/connect"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1/gatev1connect"
	"go.minekube.com/gate/pkg/util/uuid"
)

func NewService(p *proxy.Proxy) *Service {
	return &Service{
		p: p,
	}
}

type (
	Handler = gatev1connect.GateServiceHandler

	Service struct {
		p *proxy.Proxy
	}
)

type ConcreteServerInfo struct {
	name    string
	address net.Addr
}

func (c *ConcreteServerInfo) Name() string {
	return c.name
}

func (c *ConcreteServerInfo) Addr() net.Addr {
	return c.address
}

var _ Handler = (*Service)(nil)

func (s *Service) ListServers(ctx context.Context, c *connect.Request[pb.ListServersRequest]) (*connect.Response[pb.ListServersResponse], error) {
	return connect.NewResponse(&pb.ListServersResponse{
		Servers: ServersToProto(s.p.Servers()),
	}), nil
}

func (s *Service) GetPlayer(ctx context.Context, c *connect.Request[pb.GetPlayerRequest]) (*connect.Response[pb.GetPlayerResponse], error) {
	req := c.Msg

	var player proxy.Player
	switch {
	case req.GetId() != "":
		id, err := uuid.Parse(req.GetId())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid player id: %v", err))
		}
		player = s.p.Player(id)
	case req.GetUsername() != "":
		player = s.p.PlayerByName(req.GetUsername())
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id or username must be set"))
	}

	if player == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("player not found"))
	}

	return connect.NewResponse(&pb.GetPlayerResponse{
		Player: PlayerToProto(player),
	}), nil
}

func (s *Service) AddServer(ctx context.Context, c *connect.Request[pb.AddServerRequest]) (*connect.Response[pb.GetServerResponse], error) {
	name := c.Msg.GetName()
	address := c.Msg.GetAddress()

	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}

	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address is required"))
	}

	serverAddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid address: %v", err))
	}

	serverInfo := &ConcreteServerInfo{
		name:    name,
		address: serverAddr,
	}

	registeredServer, err := s.p.Register(serverInfo)
	if err != nil {
		if errors.Is(err, proxy.ErrServerAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("server %s already exists", name))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to add server: %v", err))
	}

	return connect.NewResponse(&pb.GetServerResponse{
		Server: &pb.ServerAddition{
			Name:    registeredServer.ServerInfo().Name(),
			Address: registeredServer.ServerInfo().Addr().String(),
		},
	}), nil
}

func (s *Service) RemoveServer(ctx context.Context, c *connect.Request[pb.RemoveServerRequest]) (*connect.Response[pb.RemoveServerResponse], error) {
	name := c.Msg.GetName()
	address := c.Msg.GetAddress()

	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}

	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address is required"))
	}

	serverAddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid address: %v", err))
	}

	serverInfo := &ConcreteServerInfo{
		name:    name,
		address: serverAddr,
	}

	success := s.p.Unregister(serverInfo)
	if !success {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("server %s with address %s not found", name, address))
	}

	return connect.NewResponse(&pb.RemoveServerResponse{
		Server: &pb.ServerRemoval{
			Name:    name,
			Address: address,
		},
	}), nil
}

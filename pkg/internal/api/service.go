package api

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/key"

	"go.minekube.com/gate/pkg/edition/java/cookie"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1/gatev1connect"
	"go.minekube.com/gate/pkg/util/componentutil"
	"go.minekube.com/gate/pkg/util/netutil"
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

var _ Handler = (*Service)(nil)

func (s *Service) ListPlayers(ctx context.Context, c *connect.Request[pb.ListPlayersRequest]) (*connect.Response[pb.ListPlayersResponse], error) {
	var players []proxy.Player
	if len(c.Msg.Servers) == 0 {
		players = s.p.Players()
	} else {
		for _, svr := range c.Msg.Servers {
			if s := s.p.Server(svr); s != nil {
				s.Players().Range(func(p proxy.Player) bool {
					players = append(players, p)
					return true
				})
			}
		}
	}
	return connect.NewResponse(&pb.ListPlayersResponse{
		Players: PlayersToProto(players),
	}), nil
}

func (s *Service) RegisterServer(ctx context.Context, c *connect.Request[pb.RegisterServerRequest]) (*connect.Response[pb.RegisterServerResponse], error) {
	serverAddr := netutil.NewAddr(c.Msg.Address, "tcp")
	serverInfo := proxy.NewServerInfo(c.Msg.Name, serverAddr)

	_, err := s.p.Register(serverInfo)
	if err != nil {
		if errors.Is(err, proxy.ErrServerAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("server %q already exists", serverInfo.Name()))
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid server info: %v", err))
	}

	return connect.NewResponse(&pb.RegisterServerResponse{}), nil
}

func (s *Service) UnregisterServer(ctx context.Context, c *connect.Request[pb.UnregisterServerRequest]) (*connect.Response[pb.UnregisterServerResponse], error) {
	var serverInfo proxy.ServerInfo

	switch {
	case c.Msg.Name != "" && c.Msg.Address != "":
		serverAddr := netutil.NewAddr(c.Msg.Address, "tcp")
		serverInfo = proxy.NewServerInfo(c.Msg.Name, serverAddr)
	case c.Msg.Name != "":
		if s := s.p.Server(c.Msg.Name); s != nil {
			serverInfo = s.ServerInfo()
		} else {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found by name"))
		}
	case c.Msg.Address != "":
		var found bool
		for _, s := range s.p.Servers() {
			if s.ServerInfo().Addr().String() == c.Msg.Address {
				serverInfo = s.ServerInfo()
				found = true
				break
			}
		}
		if !found {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found by address"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid request: must specify either name and/or address"))
	}

	found := s.p.Unregister(serverInfo)
	if !found {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("server not found with name %q and address %q", serverInfo.Name(), serverInfo.Addr()))
	}

	return connect.NewResponse(&pb.UnregisterServerResponse{}), nil
}

func (s *Service) ConnectPlayer(ctx context.Context, c *connect.Request[pb.ConnectPlayerRequest]) (*connect.Response[pb.ConnectPlayerResponse], error) {
	var player proxy.Player
	if id, err := uuid.Parse(c.Msg.Player); err == nil {
		player = s.p.Player(id)
	} else {
		player = s.p.PlayerByName(c.Msg.Player)
	}
	if player == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("player not found"))
	}

	targetServer := s.p.Server(c.Msg.Server)
	if targetServer == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("server not found"))
	}

	connectionRequest := player.CreateConnectionRequest(targetServer)
	_, err := connectionRequest.Connect(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	return connect.NewResponse(&pb.ConnectPlayerResponse{}), nil
}

func (s *Service) DisconnectPlayer(ctx context.Context, c *connect.Request[pb.DisconnectPlayerRequest]) (*connect.Response[pb.DisconnectPlayerResponse], error) {
	var player proxy.Player
	if id, err := uuid.Parse(c.Msg.Player); err == nil {
		player = s.p.Player(id)
	} else {
		player = s.p.PlayerByName(c.Msg.Player)
	}

	if player == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("player not found"))
	}

	var reason *component.Text
	if c.Msg.Reason != "" {
		var err error
		reason, err = componentutil.ParseTextComponent(player.Protocol(), c.Msg.Reason)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("could not parse reason: %v", err))
		}
	}

	player.Disconnect(reason)

	return connect.NewResponse(&pb.DisconnectPlayerResponse{}), nil
}

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

func (s *Service) RequestCookie(ctx context.Context, c *connect.Request[pb.RequestCookieRequest]) (*connect.Response[pb.RequestCookieResponse], error) {
	var player proxy.Player
	if id, err := uuid.Parse(c.Msg.Player); err == nil {
		player = s.p.Player(id)
	} else {
		player = s.p.PlayerByName(c.Msg.Player)
	}
	if player == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("player not found"))
	}

	key, err := key.Parse(c.Msg.Key)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid key: %v", err))
	}

	cookie, err := cookie.Request(ctx, player, key, s.p.Event())
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	return connect.NewResponse(&pb.RequestCookieResponse{
		Payload: cookie.Payload,
	}), nil
}

func (s *Service) StoreCookie(ctx context.Context, c *connect.Request[pb.StoreCookieRequest]) (*connect.Response[pb.StoreCookieResponse], error) {
	var player proxy.Player
	if id, err := uuid.Parse(c.Msg.Player); err == nil {
		player = s.p.Player(id)
	} else {
		player = s.p.PlayerByName(c.Msg.Player)
	}
	if player == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("player not found"))
	}

	key, err := key.Parse(c.Msg.Key)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid key: %v", err))
	}

	err = cookie.Store(player, &cookie.Cookie{
		Key:     key,
		Payload: c.Msg.Payload,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	return connect.NewResponse(&pb.StoreCookieResponse{}), nil
}

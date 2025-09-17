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

// ConfigHandler defines methods for configuration management
type ConfigHandler interface {
	GetStatus(context.Context, *pb.GetStatusRequest) (*pb.GetStatusResponse, error)
	GetConfig(context.Context, *pb.GetConfigRequest) (*pb.GetConfigResponse, error)
	ValidateConfig(context.Context, *pb.ValidateConfigRequest) ([]string, error)
	ApplyConfig(context.Context, *pb.ApplyConfigRequest) ([]string, error)
}

// LiteHandler defines methods for lite route management
type LiteHandler interface {
	ListLiteRoutes(context.Context, *pb.ListLiteRoutesRequest) (*pb.ListLiteRoutesResponse, error)
	GetLiteRoute(context.Context, *pb.GetLiteRouteRequest) (*pb.GetLiteRouteResponse, error)
	UpdateLiteRouteStrategy(context.Context, *pb.UpdateLiteRouteStrategyRequest) ([]string, error)
	AddLiteRouteBackend(context.Context, *pb.AddLiteRouteBackendRequest) ([]string, error)
	RemoveLiteRouteBackend(context.Context, *pb.RemoveLiteRouteBackendRequest) ([]string, error)
	UpdateLiteRouteOptions(context.Context, *pb.UpdateLiteRouteOptionsRequest) ([]string, error)
	UpdateLiteRouteFallback(context.Context, *pb.UpdateLiteRouteFallbackRequest) ([]string, error)
}

func NewService(p *proxy.Proxy, configHandler ConfigHandler, liteHandler LiteHandler) *Service {
	return &Service{
		p:             p,
		configHandler: configHandler,
		liteHandler:   liteHandler,
	}
}

type (
	Handler = gatev1connect.GateServiceHandler

	Service struct {
		p             *proxy.Proxy
		configHandler ConfigHandler
		liteHandler   LiteHandler
	}
)

var (
	_ Handler                              = (*Service)(nil)
	_ gatev1connect.GateLiteServiceHandler = (*Service)(nil)
)

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

func (s *Service) GetStatus(ctx context.Context, c *connect.Request[pb.GetStatusRequest]) (*connect.Response[pb.GetStatusResponse], error) {
	if s.configHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("config handler not configured"))
	}
	resp, err := s.configHandler.GetStatus(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Service) GetConfig(ctx context.Context, c *connect.Request[pb.GetConfigRequest]) (*connect.Response[pb.GetConfigResponse], error) {
	if s.configHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("config handler not configured"))
	}
	resp, err := s.configHandler.GetConfig(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Service) ValidateConfig(ctx context.Context, c *connect.Request[pb.ValidateConfigRequest]) (*connect.Response[pb.ValidateConfigResponse], error) {
	if s.configHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("config handler not configured"))
	}
	warns, err := s.configHandler.ValidateConfig(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.ValidateConfigResponse{Warnings: warns}), nil
}

func (s *Service) ApplyConfig(ctx context.Context, c *connect.Request[pb.ApplyConfigRequest]) (*connect.Response[pb.ApplyConfigResponse], error) {
	if s.configHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("config handler not configured"))
	}
	warns, err := s.configHandler.ApplyConfig(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.ApplyConfigResponse{Warnings: warns}), nil
}

func (s *Service) ListLiteRoutes(ctx context.Context, c *connect.Request[pb.ListLiteRoutesRequest]) (*connect.Response[pb.ListLiteRoutesResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	resp, err := s.liteHandler.ListLiteRoutes(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Service) GetLiteRoute(ctx context.Context, c *connect.Request[pb.GetLiteRouteRequest]) (*connect.Response[pb.GetLiteRouteResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	resp, err := s.liteHandler.GetLiteRoute(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Service) UpdateLiteRouteStrategy(ctx context.Context, c *connect.Request[pb.UpdateLiteRouteStrategyRequest]) (*connect.Response[pb.UpdateLiteRouteStrategyResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	warns, err := s.liteHandler.UpdateLiteRouteStrategy(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.UpdateLiteRouteStrategyResponse{Warnings: warns}), nil
}

func (s *Service) AddLiteRouteBackend(ctx context.Context, c *connect.Request[pb.AddLiteRouteBackendRequest]) (*connect.Response[pb.AddLiteRouteBackendResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	warns, err := s.liteHandler.AddLiteRouteBackend(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.AddLiteRouteBackendResponse{Warnings: warns}), nil
}

func (s *Service) RemoveLiteRouteBackend(ctx context.Context, c *connect.Request[pb.RemoveLiteRouteBackendRequest]) (*connect.Response[pb.RemoveLiteRouteBackendResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	warns, err := s.liteHandler.RemoveLiteRouteBackend(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.RemoveLiteRouteBackendResponse{Warnings: warns}), nil
}

func (s *Service) UpdateLiteRouteOptions(ctx context.Context, c *connect.Request[pb.UpdateLiteRouteOptionsRequest]) (*connect.Response[pb.UpdateLiteRouteOptionsResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	warns, err := s.liteHandler.UpdateLiteRouteOptions(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.UpdateLiteRouteOptionsResponse{Warnings: warns}), nil
}

func (s *Service) UpdateLiteRouteFallback(ctx context.Context, c *connect.Request[pb.UpdateLiteRouteFallbackRequest]) (*connect.Response[pb.UpdateLiteRouteFallbackResponse], error) {
	if s.liteHandler == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("lite handler not configured"))
	}
	warns, err := s.liteHandler.UpdateLiteRouteFallback(ctx, c.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.UpdateLiteRouteFallbackResponse{Warnings: warns}), nil
}

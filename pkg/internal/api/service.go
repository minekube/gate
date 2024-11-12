package api

import (
	"context"
	"errors"
	"fmt"

	pb "buf.build/gen/go/minekube/gate/protocolbuffers/go/minekube/gate"

	"buf.build/gen/go/minekube/gate/connectrpc/go/minekube/gate/gatev1connect"
	"connectrpc.com/connect"

	"go.minekube.com/gate/pkg/edition/java/proxy"
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

var _ Handler = (*Service)(nil)

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

func (s *Service) UpdateServers(ctx context.Context, c *connect.Request[pb.UpdateServersRequest]) (*connect.Response[pb.UpdateServersResponse], error) {
	var responses []*pb.ServerResponse
	var err error

	for _, op := range c.Msg.Operations {

		switch op.Operation {
		case pb.Operation_OPERATION_CREATE:

			serverAddr := netutil.NewAddr(op.Address, "tcp")

			serverInfo := &ConcreteServerInfo{
				name:    op.Name,
				address: serverAddr,
			}

			_, err = s.p.Register(serverInfo)
			if err != nil {
				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      false,
					ErrorMessage: fmt.Sprintf("failed to add server: %v", err),
				})
			} else {
				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      true,
					ErrorMessage: "",
				})
			}

		case pb.Operation_OPERATION_DELETE:
			var serverInfo *ConcreteServerInfo

			if op.Name != "" && op.Address != "" {
				serverAddr := netutil.NewAddr(op.Address, "tcp")
				serverInfo = &ConcreteServerInfo{
					name:    op.Name,
					address: serverAddr,
				}

			} else if op.Name != "" {
				registeredServer := s.p.Server(op.Name)
				if registeredServer != nil {
					var ok bool
					serverInfo, ok = registeredServer.ServerInfo().(*ConcreteServerInfo)
					if !ok {
						responses = append(responses, &pb.ServerResponse{
							Name:         op.Name,
							Address:      op.Address,
							Success:      false,
							ErrorMessage: "server info type mismatch",
						})
						continue
					}
				} else {
					responses = append(responses, &pb.ServerResponse{
						Name:         op.Name,
						Address:      op.Address,
						Success:      false,
						ErrorMessage: "server not found by name",
					})
					continue
				}
			} else if op.Address != "" {
				var found bool
				registeredServers := s.p.Servers()
				for _, registeredServer := range registeredServers {
					if registeredServer.ServerInfo().Addr().String() == op.Address {
						var ok bool
						serverInfo, ok = registeredServer.ServerInfo().(*ConcreteServerInfo)
						if !ok {
							responses = append(responses, &pb.ServerResponse{
								Name:         op.Name,
								Address:      op.Address,
								Success:      false,
								ErrorMessage: "server info type mismatch",
							})
							continue
						}
						found = true
						break
					}
				}

				if !found {
					responses = append(responses, &pb.ServerResponse{
						Name:         op.Name,
						Address:      op.Address,
						Success:      false,
						ErrorMessage: "server not found by address",
					})
					continue
				}

			} else {
				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      false,
					ErrorMessage: "invalid request: must specify either name or address",
				})
			}

			success := s.p.Unregister(serverInfo)
			if !success {
				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      false,
					ErrorMessage: "failed to remove server",
				})
			} else {
				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      true,
					ErrorMessage: "",
				})
			}

		case pb.Operation_OPERATION_UNSPECIFIED:
			serverAddr := netutil.NewAddr(op.Address, "tcp")

			serverInfo := &ConcreteServerInfo{
				name:    op.Name,
				address: serverAddr,
			}

			registeredServer := s.p.Server(op.Name)

			if registeredServer != nil {
				if registeredServer.ServerInfo() != nil && registeredServer.ServerInfo().Addr() != nil {
					if registeredServer.ServerInfo().Addr().String() == serverInfo.address.String() {
						responses = append(responses, &pb.ServerResponse{
							Name:         op.Name,
							Address:      op.Address,
							Success:      true,
							ErrorMessage: "failed to update server: already matches the existing address",
						})
						continue
					}
				} else {
					responses = append(responses, &pb.ServerResponse{
						Name:         op.Name,
						Address:      op.Address,
						Success:      false,
						ErrorMessage: "server info or address is nil",
					})
					continue
				}

				success := s.p.Unregister(registeredServer.ServerInfo())
				if !success {
					responses = append(responses, &pb.ServerResponse{
						Name:         op.Name,
						Address:      op.Address,
						Success:      false,
						ErrorMessage: "failed to remove existing server",
					})
					continue
				}

				_, err := s.p.Register(serverInfo)
				if err != nil {
					responses = append(responses, &pb.ServerResponse{
						Name:         op.Name,
						Address:      op.Address,
						Success:      false,
						ErrorMessage: fmt.Sprintf("failed to re-add server: %v", err),
					})
					continue
				}

				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      true,
					ErrorMessage: "",
				})

			} else {
				_, err := s.p.Register(serverInfo)
				if err != nil {
					responses = append(responses, &pb.ServerResponse{
						Name:         op.Name,
						Address:      op.Address,
						Success:      false,
						ErrorMessage: fmt.Sprintf("failed to add new server: %v", err),
					})
					continue
				}

				responses = append(responses, &pb.ServerResponse{
					Name:         op.Name,
					Address:      op.Address,
					Success:      true,
					ErrorMessage: "",
				})
			}
		}
	}
	return connect.NewResponse(&pb.UpdateServersResponse{Responses: responses}), nil
}

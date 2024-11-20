package api

import (
	"go.minekube.com/gate/pkg/edition/java/proxy"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func PlayersToProto(p []proxy.Player) []*pb.Player {
	var players []*pb.Player
	for _, player := range p {
		players = append(players, PlayerToProto(player))
	}
	return players
}

func PlayerToProto(p proxy.Player) *pb.Player {
	return &pb.Player{
		Id:       p.ID().String(),
		Username: p.Username(),
	}
}

func ServersToProto(s []proxy.RegisteredServer) []*pb.Server {
	var servers []*pb.Server
	for _, server := range s {
		servers = append(servers, ServerToProto(server))
	}
	return servers
}

func ServerToProto(s proxy.RegisteredServer) *pb.Server {
	return &pb.Server{
		Name:    s.ServerInfo().Name(),
		Address: s.ServerInfo().Addr().String(),
		Players: int32(s.Players().Len()),
	}
}

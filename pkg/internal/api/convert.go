package api

import (
	"go.minekube.com/gate/pkg/edition/java/proxy"
	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
)

func PlayerToProto(p proxy.Player) *pb.Player {
	return &pb.Player{
		Id:       p.ID().String(),
		Username: p.Username(),
	}
}

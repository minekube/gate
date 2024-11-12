package api

import (
	pb "buf.build/gen/go/minekube/gate/protocolbuffers/go/minekube/gate"

	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func PlayerToProto(p proxy.Player) *pb.Player {
	return &pb.Player{
		Id:       p.ID().String(),
		Username: p.Username(),
	}
}

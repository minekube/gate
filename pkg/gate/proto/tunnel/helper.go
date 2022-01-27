package tunnel

import (
	"context"
	"errors"
	"fmt"
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"go.minekube.com/gate/pkg/util/netutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"time"
)

func Watch(ctx context.Context, endpoint string, connectCli pb.ConnectServiceClient, fn func(session *pb.Session) error) error {
	stream, err := connectCli.Watch(ctx, &pb.WatchRequest{Endpoint: &pb.Endpoint{Name: endpoint}})
	if err != nil {
		return err
	}
	for {
		start, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if start.GetSession() == nil {
			continue
		}
		if err = fn(start.GetSession()); err != nil {
			return err
		}
	}
}

func Tunnel(ctx context.Context, session *pb.Session) (Conn, error) {
	dialTimeout, dialCancel := context.WithTimeout(ctx, time.Second*10)           // todo config timeout
	svcConn, err := grpc.DialContext(dialTimeout, session.GetTunnelServiceAddr(), // TODO config target
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	dialCancel()
	if err != nil {
		return nil, fmt.Errorf("could not dial tunnel service at %q: %w", session.GetTunnelServiceAddr(), err)
	}
	tunnelCli := pb.NewTunnelServiceClient(svcConn)

	tunnelCtx, tunnelCancel := context.WithCancel(ctx)
	tunnelStream, err := tunnelCli.Tunnel(tunnelCtx)
	if err != nil {
		tunnelCancel()
		_ = svcConn.Close()
		return nil, fmt.Errorf("error starting tunnel: %w", err)
	}

	localAddr := netutil.NewAddr("tcp", "localhost", 0)
	remoteAddr, _ := netutil.Parse(session.GetTunnelServiceAddr(), "grpc")
	r, w := clientStreamRW(tunnelStream)
	return newConn(session, localAddr, remoteAddr, r, w, func(err error) {
		tunnelCancel()
		_ = svcConn.Close()
	}), nil
}

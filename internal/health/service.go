// GRPC health check server
// (https://godoc.org/google.golang.org/grpc/health/grpc_health_v1)
package health

import (
	"context"
	"google.golang.org/grpc"
	rpc "google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"time"
)

func New(addr string) (run func(ctx context.Context, checkFn CheckFn) error, err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, checkFn CheckFn) error {
		s := grpc.NewServer(grpc.ConnectionTimeout(time.Second * 3))
		rpc.RegisterHealthServer(s, &server{checkFn: checkFn})
		go func() {
			<-ctx.Done()
			s.Stop()
		}()
		return s.Serve(ln)
	}, nil
}

type CheckFn func(ctx context.Context) (*rpc.HealthCheckResponse, error)

type server struct {
	rpc.UnimplementedHealthServer
	checkFn CheckFn
}

func (s *server) Check(ctx context.Context, _ *rpc.HealthCheckRequest) (*rpc.HealthCheckResponse, error) {
	return s.checkFn(ctx)
}

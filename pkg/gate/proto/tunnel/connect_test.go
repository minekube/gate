package tunnel

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	pb "go.minekube.com/gate/pkg/gate/proto/tunnel/pb"
	"go.minekube.com/gate/pkg/runtime/event"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"io"
	"sync"
	"testing"
	"time"
)

var samplePlayer = &pb.Player{
	Profile: &pb.GameProfile{
		Id:         uuid.New().String(),
		Name:       "SamplePlayer",
		Properties: nil,
	},
}

const listenAddr = ":8443"

var (
	startOnce sync.Once
	ctx, _    = context.WithTimeout(context.Background(), time.Minute*10)
	c         = &Connect{
		Event: event.New(nil),
	}
)

func startLocalConnectServices(t testing.TB) {
	startOnce.Do(func() {
		go func() {
			err := c.ListenAndServer(ctx, listenAddr, listenAddr)
			if errors.Is(err, context.Canceled) {
				return
			}
			require.NoError(t, err)
		}()
		time.Sleep(time.Second)
	})
}

func TestConnect_Dial_NoActiveTunnel(t *testing.T) {
	startLocalConnectServices(t)

	_, err := c.Dial(ctx, "not-exists", samplePlayer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active tunnel")
}

func TestService_Tunnel(t *testing.T) {
	startLocalConnectServices(t)

	svcConn, err := grpc.Dial(listenAddr, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer svcConn.Close()

	tunnelCli := pb.NewTunnelServiceClient(svcConn)

	biStream, err := tunnelCli.Tunnel(ctx)
	require.NoError(t, err)

	err = biStream.Send(&pb.TunnelRequest{})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 300)

	err = biStream.Send(&pb.TunnelRequest{})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, status.Code(biStream.RecvMsg(nil)), codes.InvalidArgument)
}

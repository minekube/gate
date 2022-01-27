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
	"net"
	"os"
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
			err := c.ListenAndServe(ctx, listenAddr, listenAddr)
			if errors.Is(err, context.Canceled) {
				return
			}
			require.NoError(t, err)
		}()
		time.Sleep(time.Millisecond * 300)
	})
}

var (
	setupClientsOnce sync.Once
	tunnelCli        pb.TunnelServiceClient
	connectCli       pb.ConnectServiceClient
)

func setupClients(t testing.TB) {
	setupClientsOnce.Do(func() {
		svcConn, err := grpc.Dial(listenAddr, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		tunnelCli = pb.NewTunnelServiceClient(svcConn)
		connectCli = pb.NewConnectServiceClient(svcConn)
	})
}

func TestService_Tunnel_SessionID_Missing(t *testing.T) {
	startLocalConnectServices(t)
	setupClients(t)

	biStream, err := tunnelCli.Tunnel(ctx)
	require.NoError(t, err)

	err = biStream.Send(&pb.TunnelRequest{
		Message: &pb.TunnelRequest_SessionId{
			SessionId: "", // missing
		},
	})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 300)

	err = biStream.Send(&pb.TunnelRequest{})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, status.Code(biStream.RecvMsg(nil)), codes.InvalidArgument)
}

func TestService_Tunnel_SessionID_NotFound(t *testing.T) {
	startLocalConnectServices(t)
	setupClients(t)

	biStream, err := tunnelCli.Tunnel(ctx)
	require.NoError(t, err)

	err = biStream.Send(&pb.TunnelRequest{
		Message: &pb.TunnelRequest_SessionId{
			SessionId: "not-existing",
		},
	})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 300)

	err = biStream.Send(&pb.TunnelRequest{})
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, status.Code(biStream.RecvMsg(nil)), codes.NotFound)
}

func TestService_Watch_Endpoint_Missing(t *testing.T) {
	startLocalConnectServices(t)
	setupClients(t)

	stream, err := connectCli.Watch(ctx, &pb.WatchRequest{
		Endpoint: nil, // missing
	})
	require.NoError(t, err)

	_, err = stream.Recv()
	require.Equal(t, status.Code(err), codes.InvalidArgument)
}

func TestConnect_Dial_NoActiveTunnel(t *testing.T) {
	startLocalConnectServices(t)

	_, err := c.Dial(ctx, "not-existing", samplePlayer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active tunnel")
}

const sampleEndpoint = "sampleEndpoint"

func TestService_Watch_Dial_Tunnel_RW(t *testing.T) {
	startLocalConnectServices(t)
	setupClients(t)

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	watchStream, err := connectCli.Watch(watchCtx, &pb.WatchRequest{
		Endpoint: &pb.Endpoint{Name: sampleEndpoint},
	})
	require.NoError(t, err)
	time.Sleep(time.Millisecond * 300)

	tunnelChan := make(chan net.Conn, 1)
	go func() {
		// Trigger tunnel creation
		tunnelConn, err := c.Dial(ctx, sampleEndpoint, samplePlayer) // server bound
		require.NoError(t, err)
		tunnelChan <- tunnelConn
	}()

	resp, err := watchStream.Recv()
	require.NoError(t, err)

	// Test watching endpoint is removed
	watchCancel()
	time.Sleep(time.Millisecond * 300)
	_, err = c.Dial(ctx, sampleEndpoint, samplePlayer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active tunnel")

	// Create tunnel
	session := resp.GetSession()
	tunnelCtx, tunnelCancel := context.WithCancel(ctx)
	defer tunnelCancel()
	tunnelStream, err := tunnelCli.Tunnel(tunnelCtx) // client bound
	require.NoError(t, err)

	err = tunnelStream.Send(&pb.TunnelRequest{Message: &pb.TunnelRequest_SessionId{
		SessionId: session.GetId(),
	}})
	require.NoError(t, err)

	err = tunnelStream.Send(&pb.TunnelRequest{Message: &pb.TunnelRequest_Data{
		Data: []byte("hello"),
	}})
	require.NoError(t, err)

	// Test tunnel is still active and read from it
	tunnelConn := <-tunnelChan
	defer tunnelConn.Close()
	b := make([]byte, 2)
	n, err := tunnelConn.Read(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)
	require.Equal(t, "he", string(b))

	b = make([]byte, 3)
	n, err = tunnelConn.Read(b)
	require.NoError(t, err)
	require.Equal(t, len(b), n)
	require.Equal(t, "llo", string(b))

	// Write to tunnel
	writeB := []byte("holla")
	n, err = tunnelConn.Write(writeB)
	require.NoError(t, err)
	require.Equal(t, 5, n)

	req, err := tunnelStream.Recv()
	require.NoError(t, err)
	require.Equal(t, writeB, req.GetData())

	// Deadline
	ok := make(chan struct{})
	go func() {
		defer close(ok)
		err = tunnelConn.SetDeadline(time.Now().Add(time.Second))
		require.NoError(t, err)
		_, err = tunnelConn.Read(make([]byte, 3))
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	}()
	select {
	case <-ok:
	case <-time.After(time.Second + time.Millisecond*100):
		require.Fail(t, "deadline did not exceed")
	}
}

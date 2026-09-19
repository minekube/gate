package api

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	pb "go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1"
	"go.minekube.com/gate/pkg/internal/api/gen/minekube/gate/v1/gatev1connect"
)

// The API endpoint is served without TLS, so gRPC/Connect clients rely on
// cleartext HTTP/2 (h2c, prior knowledge). This guarded behaviour used to come
// from the golang.org/x/net/http2/h2c wrapper; it is now provided by
// http.Server.Protocols. Without Protocols.UnencryptedHTTP2 a prior-knowledge
// HTTP/2 client can no longer reach the API.
func TestServerServesUnencryptedHTTP2(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.Handle(gatev1connect.NewGateServiceHandler(NewService(nil, &testConfigHandler{})))
	srv := newHTTPServer("", mux, context.Background())
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	client := gatev1connect.NewGateServiceClient(
		&http.Client{Transport: &http.Transport{Protocols: protocols}},
		"http://"+ln.Addr().String(),
		connect.WithGRPC(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := client.GetStatus(ctx, connect.NewRequest(&pb.GetStatusRequest{}))
	require.NoError(t, err, "gRPC over cleartext HTTP/2 (prior knowledge) must be served")
	require.Equal(t, "test-version", res.Msg.GetVersion())

	// Plain HTTP/1.1 clients keep working as well.
	http1 := &http.Client{Timeout: 10 * time.Second}
	resp, err := http1.Get("http://" + ln.Addr().String() + "/minekube.gate.v1.GateService/GetStatus")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, "HTTP/1.1", resp.Proto)
}

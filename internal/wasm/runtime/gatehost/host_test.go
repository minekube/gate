package gatehost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/api"
	"go.minekube.com/gate/internal/wasm/runtime/wire"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestInvokeCallsGeneratedGateAPIWithRealProxyResource(t *testing.T) {
	t.Parallel()

	host, err := New("fixture", context.Background(), &proxy.Proxy{}, 16)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, host.Close()) })

	request, err := wire.Encode([]any{wire.Resource(host.ProxyHandle())})
	require.NoError(t, err)
	responseBytes, err := host.Invoke(playerCountOperationID(t), request)
	require.NoError(t, err)
	response, err := wire.DecodeResponse(responseBytes)
	require.NoError(t, err)
	require.Nil(t, response.Error)
	require.Equal(t, []any{int64(0)}, response.Values)
}

func TestInvokeReturnsStructuredGateError(t *testing.T) {
	t.Parallel()

	host, err := New("fixture", context.Background(), &proxy.Proxy{}, 16)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, host.Close()) })

	request, err := wire.Encode(nil)
	require.NoError(t, err)
	responseBytes, err := host.Invoke(playerCountOperationID(t), request)
	require.NoError(t, err)
	response, err := wire.DecodeResponse(responseBytes)
	require.NoError(t, err)
	require.NotNil(t, response.Error)
	require.Equal(t, "go.minekube.com/gate/pkg/edition/java/proxy.Proxy.PlayerCount", response.Error.Operation)
	require.Contains(t, response.Error.Message, "want 1")
}

func playerCountOperationID(t *testing.T) uint32 {
	t.Helper()
	const identity = "go.minekube.com/gate/pkg/edition/java/proxy.Proxy.PlayerCount"
	for _, operation := range api.GeneratedOperations {
		if operation.Identity == identity {
			return operation.ID
		}
	}
	t.Fatalf("generated operation %q is missing", identity)
	return 0
}

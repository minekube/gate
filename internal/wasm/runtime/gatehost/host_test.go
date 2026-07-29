package gatehost

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/api"
	"go.minekube.com/gate/internal/wasm/runtime/dispatch"
	"go.minekube.com/gate/internal/wasm/runtime/wire"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestContextRuntimeExtensionsExposeCancellationAndDeadline(t *testing.T) {
	t.Parallel()

	deadline := time.Now().Add(time.Minute).Round(0)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	host, err := New("fixture", ctx, &proxy.Proxy{}, 16)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, host.Close()) })

	results, err := host.invokeExtension(ctx, dispatch.Operation{
		Identity: "go.minekube.com/gate/pkg/gate#wasm-context-deadline",
	}, []any{ctx})
	require.NoError(t, err)
	require.Equal(t, []any{deadline.UnixNano(), true}, results)

	results, err = host.invokeExtension(ctx, dispatch.Operation{
		Identity: "go.minekube.com/gate/pkg/gate#wasm-context-cancelled",
	}, []any{ctx})
	require.NoError(t, err)
	require.Equal(t, []any{false}, results)

	cancel()
	results, err = host.invokeExtension(ctx, dispatch.Operation{
		Identity: "go.minekube.com/gate/pkg/gate#wasm-context-error",
	}, []any{ctx})
	require.NoError(t, err)
	require.Equal(t, []any{context.Canceled.Error()}, results)
}

func TestContextLogRuntimeExtensionValidatesStructuredFields(t *testing.T) {
	t.Parallel()

	host, err := New("fixture", context.Background(), &proxy.Proxy{}, 16)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, host.Close()) })

	_, err = host.invokeExtension(context.Background(), dispatch.Operation{
		Identity: "go.minekube.com/gate/pkg/gate#wasm-log",
	}, []any{context.Background(), int64(0), "hello", []string{"key"}})
	require.ErrorContains(t, err, "key/value pairs")
}

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

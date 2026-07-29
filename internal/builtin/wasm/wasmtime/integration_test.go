//go:build wasm_native && cgo

package native_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/host"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/resources"
	"go.minekube.com/gate/internal/builtin/wasm/wasmtime"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestRuntimeInitReceivesContextAndRealProxy(t *testing.T) {
	component, err := os.ReadFile("artifacts/gate_wasm_fixture.component.wasm")
	require.NoError(t, err)
	before := resources.LiveResources()
	host, err := gatehost.New(
		"gate-wasm-fixture",
		context.Background(),
		&proxy.Proxy{},
		64,
	)
	require.NoError(t, err)
	runtime, err := native.New(component, host, native.Limits{})
	require.NoError(t, err)
	metadata, err := runtime.Metadata()
	require.NoError(t, err)
	require.Equal(t, "gate-wasm-fixture", metadata.Name)
	require.Equal(t, "0.0.0", metadata.Version)
	require.Len(t, metadata.ContractHash, 64)
	require.EqualValues(t, 1, metadata.GeneratorFormat)

	err = runtime.Init(host.ContextHandle(), host.ProxyHandle())
	require.NoError(t, err)

	require.NoError(t, runtime.Close())
	require.NoError(t, runtime.Close())
	require.NoError(t, host.Close())
	require.Equal(t, before, resources.LiveResources())
}

func TestRuntimeRejectsInvalidComponent(t *testing.T) {
	host, err := gatehost.New(
		"invalid",
		context.Background(),
		&proxy.Proxy{},
		8,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, host.Close()) })

	_, err = native.New([]byte("not wasm"), host, native.Limits{})
	require.ErrorContains(t, err, "failed to compile component")
}

//go:build wasm_native && wasm_example && cgo

package native_test

import (
	"context"
	"os"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/runtime/gatehost"
	"go.minekube.com/gate/internal/wasm/runtime/native"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

func TestPublicRustExampleLoadsAndInitializes(t *testing.T) {
	component, err := os.ReadFile(
		"../../../../.examples/wasm/rust/gate-rust-example.component.wasm",
	)
	require.NoError(t, err)

	cfg := config.DefaultConfig
	gateProxy, err := proxy.New(proxy.Options{
		Config:   &cfg,
		EventMgr: event.New(),
	})
	require.NoError(t, err)
	host, err := gatehost.New(
		"gate-rust-example",
		context.Background(),
		gateProxy,
		128,
	)
	require.NoError(t, err)
	runtime, err := native.New(component, host, native.Limits{})
	require.NoError(t, err)
	metadata, err := runtime.Metadata()
	require.NoError(t, err)
	require.Equal(t, "gate-rust-example", metadata.Name)

	require.NoError(t, runtime.Init(host.ContextHandle(), host.ProxyHandle()))
	host.StopRegistrations()
	require.NoError(t, runtime.Close())
	require.NoError(t, host.Close())
}

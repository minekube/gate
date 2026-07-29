package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type fakeManagerLifecycle struct {
	ctx      context.Context
	proxy    *proxy.Proxy
	startErr error
	closed   int
}

func (manager *fakeManagerLifecycle) Start(ctx context.Context, gateProxy *proxy.Proxy) error {
	manager.ctx = ctx
	manager.proxy = gateProxy
	return manager.startErr
}

func (manager *fakeManagerLifecycle) Close() error {
	manager.closed++
	return nil
}

func TestPluginUsesNativeInitAndShutdownLifecycle(t *testing.T) {
	events := event.New()
	cfg := config.DefaultConfig
	gateProxy, err := proxy.New(proxy.Options{Config: &cfg, EventMgr: events})
	require.NoError(t, err)
	manager := &fakeManagerLifecycle{}
	builtin := plugin(manager)
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "same")

	require.Equal(t, "wasm", builtin.Name)
	require.NoError(t, builtin.Init(ctx, gateProxy))
	require.Same(t, ctx, manager.ctx)
	require.Same(t, gateProxy, manager.proxy)

	events.Fire(&proxy.ShutdownEvent{})
	events.Wait()
	require.Equal(t, 1, manager.closed)
}

func TestPluginReturnsManagerStartupFailure(t *testing.T) {
	expected := errors.New("start failed")
	manager := &fakeManagerLifecycle{startErr: expected}
	builtin := plugin(manager)

	require.ErrorIs(t, builtin.Init(context.Background(), &proxy.Proxy{}), expected)
	require.Zero(t, manager.closed)
}

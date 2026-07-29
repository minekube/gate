package proxy

import (
	"context"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
)

type fixtureComponentPlugins struct {
	ctx    context.Context
	proxy  *Proxy
	closed int
}

func (plugins *fixtureComponentPlugins) Start(ctx context.Context, proxy *Proxy) error {
	plugins.ctx = ctx
	plugins.proxy = proxy
	return nil
}

func (plugins *fixtureComponentPlugins) Close() error {
	plugins.closed++
	return nil
}

func TestInitPluginsRunsOneNativePluginSequence(t *testing.T) {
	var order []string
	gateProxy := &Proxy{plugins: []Plugin{
		{Name: "native", Init: func(context.Context, *Proxy) error {
			order = append(order, "native")
			return nil
		}},
		{Name: "wasm", Init: func(context.Context, *Proxy) error {
			order = append(order, "wasm")
			return nil
		}},
	}}

	require.NoError(t, gateProxy.initPlugins(context.Background()))
	require.Equal(t, []string{"native", "wasm"}, order)
}

func TestComponentManagerCompatibilityAdapterUsesNativeLifecycle(t *testing.T) {
	events := event.New()
	manager := &fixtureComponentPlugins{}
	gateProxy := &Proxy{
		event:   events,
		plugins: []Plugin{componentManagerPlugin(manager)},
	}
	ctx := context.Background()

	require.NoError(t, gateProxy.initPlugins(ctx))
	require.Same(t, gateProxy, manager.proxy)
	require.Equal(t, ctx, manager.ctx)
	events.Fire(&ShutdownEvent{})
	events.Wait()
	require.Equal(t, 1, manager.closed)
}

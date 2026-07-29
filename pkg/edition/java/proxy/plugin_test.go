package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fixtureComponentPlugins struct {
	ctx   context.Context
	proxy *Proxy
}

func (plugins *fixtureComponentPlugins) Start(ctx context.Context, proxy *Proxy) error {
	plugins.ctx = ctx
	plugins.proxy = proxy
	return nil
}

func (*fixtureComponentPlugins) Close() error {
	return nil
}

func TestInitPluginsPassesSameContextAndProxyToComponentManager(t *testing.T) {
	previous := Plugins
	Plugins = nil
	t.Cleanup(func() { Plugins = previous })

	manager := &fixtureComponentPlugins{}
	gateProxy := &Proxy{componentPlugins: manager}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "lifetime")

	require.NoError(t, gateProxy.initPlugins(ctx))
	require.Same(t, gateProxy, manager.proxy)
	require.Same(t, ctx, manager.ctx)
}

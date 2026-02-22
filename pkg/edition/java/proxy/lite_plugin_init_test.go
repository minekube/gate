package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/lite"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
)

func TestInitLitePlugins_OrderAndRuntime(t *testing.T) {
	prev := lite.Plugins
	defer func() { lite.Plugins = prev }()

	var order []string
	lite.Plugins = []lite.Plugin{
		{
			Name: "a",
			Init: func(ctx context.Context, rt *lite.Runtime) error {
				require.NotNil(t, rt)
				order = append(order, "a")
				return nil
			},
		},
		{
			Name: "b",
			Init: func(ctx context.Context, rt *lite.Runtime) error {
				require.NotNil(t, rt)
				order = append(order, "b")
				return nil
			},
		},
	}

	p := &Proxy{lite: lite.NewLite()}
	require.NoError(t, p.initLitePlugins(context.Background()))
	assert.Equal(t, []string{"a", "b"}, order)
}

func TestInitLitePlugins_Failure(t *testing.T) {
	prev := lite.Plugins
	defer func() { lite.Plugins = prev }()

	want := errors.New("boom")
	lite.Plugins = []lite.Plugin{
		{
			Name: "broken",
			Init: func(ctx context.Context, rt *lite.Runtime) error { return want },
		},
	}

	p := &Proxy{lite: lite.NewLite()}
	err := p.initLitePlugins(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
	assert.Contains(t, err.Error(), `lite plugin "broken"`)
}

func TestLitePluginInitSkippedWhenLiteDisabled(t *testing.T) {
	prev := lite.Plugins
	defer func() { lite.Plugins = prev }()

	called := false
	lite.Plugins = []lite.Plugin{
		{
			Name: "noop",
			Init: func(ctx context.Context, rt *lite.Runtime) error {
				called = true
				return nil
			},
		},
	}

	cfg := jconfig.DefaultConfig
	cfg.Lite = liteconfig.Config{Enabled: false}
	p := &Proxy{cfg: &cfg, lite: lite.NewLite()}

	if p.cfg.Lite.Enabled {
		require.NoError(t, p.initLitePlugins(context.Background()))
	}

	assert.False(t, called)
}

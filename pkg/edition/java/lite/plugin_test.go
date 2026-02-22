package lite

import (
	"context"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLitePluginsDeterministicOrder(t *testing.T) {
	prev := Plugins
	defer func() { Plugins = prev }()

	var order []string
	Plugins = []Plugin{
		{
			Name: "first",
			Init: func(ctx context.Context, rt *Runtime) error {
				require.NotNil(t, rt)
				order = append(order, "first")
				return nil
			},
		},
		{
			Name: "second",
			Init: func(ctx context.Context, rt *Runtime) error {
				require.NotNil(t, rt)
				order = append(order, "second")
				return nil
			},
		},
	}

	rt := newRuntime(event.Nop)
	for _, pl := range Plugins {
		require.NoError(t, pl.Init(context.Background(), rt))
	}

	assert.Equal(t, []string{"first", "second"}, order)
}

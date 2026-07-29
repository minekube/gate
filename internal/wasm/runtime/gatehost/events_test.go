package gatehost

import (
	"context"
	"errors"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/runtime/dispatch"
	"go.minekube.com/gate/internal/wasm/runtime/resources"
)

type transactionalEvent struct {
	Value string
}

func TestEventSubscriptionCommitsSuccessfulMutationAndUnsubscribes(t *testing.T) {
	t.Parallel()

	manager := event.New()
	host := &Host{
		dispatch:     dispatch.NewHost(resources.NewTable("events", 32)),
		eventManager: manager,
	}
	result, err := host.invokeExtension(
		context.Background(),
		dispatch.Operation{Identity: "fixture.Event#wasm-subscribe"},
		[]any{7, func(value *transactionalEvent) error {
			value.Value = "committed"
			return nil
		}},
	)
	require.NoError(t, err)
	require.Len(t, result, 1)

	value := &transactionalEvent{Value: "original"}
	manager.Fire(value)
	require.Equal(t, "committed", value.Value)

	result[0].(func())()
	value.Value = "after-unsubscribe"
	manager.Fire(value)
	require.Equal(t, "after-unsubscribe", value.Value)
}

func TestEventSubscriptionRollsBackFailedMutation(t *testing.T) {
	t.Parallel()

	manager := event.New()
	host := &Host{
		dispatch:     dispatch.NewHost(resources.NewTable("events", 32)),
		eventManager: manager,
	}
	_, err := host.invokeExtension(
		context.Background(),
		dispatch.Operation{Identity: "fixture.Event#wasm-subscribe"},
		[]any{0, func(value *transactionalEvent) error {
			value.Value = "discarded"
			return errors.New("guest failed")
		}},
	)
	require.NoError(t, err)

	value := &transactionalEvent{Value: "original"}
	manager.Fire(value)
	require.Equal(t, "original", value.Value)
	require.NoError(t, host.Close())
}

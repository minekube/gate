package dispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/runtime/resources"
)

type fixtureReceiver struct {
	prefix string
}

func (receiver *fixtureReceiver) Join(value string) (string, error) {
	if value == "error" {
		return "", errors.New("fixture failure")
	}
	return receiver.prefix + value, nil
}

func TestHostInvokesFunctionsMethodsAndErrors(t *testing.T) {
	table := resources.NewTable("fixture", 4)
	t.Cleanup(func() { require.NoError(t, table.Close()) })
	host := NewHost(table)
	function := Operation{ID: 1, Identity: "fixture.Double"}

	results, err := host.Call(
		context.Background(),
		function,
		func(value int32) (int32, error) { return value * 2, nil },
		[]any{int32(4)},
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []any{int32(8)}, results)

	handle, err := table.Insert(
		&fixtureReceiver{prefix: "gate:"},
		"fixture.Receiver",
		resources.LifetimePlugin,
		nil,
	)
	require.NoError(t, err)
	results, err = host.CallMethod(
		context.Background(),
		Operation{ID: 2, Identity: "fixture.Receiver.Join"},
		handle,
		"fixture.Receiver",
		"Join",
		[]any{"ok"},
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []any{"gate:ok"}, results)

	_, err = host.CallMethod(
		context.Background(),
		Operation{ID: 2, Identity: "fixture.Receiver.Join"},
		handle,
		"fixture.Receiver",
		"Join",
		[]any{"error"},
		false,
	)
	require.ErrorContains(t, err, "fixture.Receiver.Join: fixture failure")
}

func TestHostRecoversPanicsWithOperationIdentity(t *testing.T) {
	host := NewHost(resources.NewTable("fixture", 1))
	t.Cleanup(func() { require.NoError(t, host.Close()) })

	_, err := host.Call(
		context.Background(),
		Operation{ID: 7, Identity: "fixture.Panic"},
		func() { panic("boom") },
		nil,
		false,
	)
	require.ErrorIs(t, err, ErrHostPanic)
	require.ErrorContains(t, err, "fixture.Panic")
	require.ErrorContains(t, err, "boom")
}

func TestHostRegistryRejectsMissingAndDuplicateOperations(t *testing.T) {
	host := NewHost(resources.NewTable("fixture", 1))
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	handler := func(context.Context, *Host, []any) ([]any, error) {
		return []any{"ok"}, nil
	}
	require.NoError(t, host.Register(Operation{
		ID: 1, Identity: "fixture.One", Handler: handler,
	}))
	require.ErrorIs(t, host.Register(Operation{
		ID: 1, Identity: "fixture.Duplicate", Handler: handler,
	}), ErrDuplicateOperation)

	results, err := host.Invoke(context.Background(), 1, nil)
	require.NoError(t, err)
	require.Equal(t, []any{"ok"}, results)
	_, err = host.Invoke(context.Background(), 2, nil)
	require.ErrorIs(t, err, ErrUnknownOperation)
}

func TestHostAssignsExportedVariables(t *testing.T) {
	host := NewHost(resources.NewTable("fixture", 1))
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	value := int64(1)
	require.NoError(t, host.Assign(
		Operation{ID: 3, Identity: "fixture.Value#set"},
		&value,
		int64(9),
	))
	require.EqualValues(t, 9, value)
	require.ErrorIs(t, host.Assign(
		Operation{ID: 3, Identity: "fixture.Value#set"},
		&value,
		"wrong",
	), ErrArgumentType)
}

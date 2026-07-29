package dispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/runtime/resources"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/wire"
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

func TestHostCallsFunctionResource(t *testing.T) {
	t.Parallel()

	table := resources.NewTable("callback", 8)
	t.Cleanup(func() { require.NoError(t, table.Close()) })
	host := NewHost(table)
	handle, err := table.Insert(
		func(input string) bool { return input == "gate" },
		"fixture.Handler",
		resources.LifetimeOwned,
		nil,
	)
	require.NoError(t, err)

	results, err := host.CallResource(
		context.Background(),
		Operation{ID: 1, Identity: "fixture.Handler#call"},
		[]any{wire.Resource(handle), "gate"},
	)
	require.NoError(t, err)
	require.Equal(t, []any{true}, results)
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

type fixtureInput struct {
	HTTPServerID string
	Values       []int32
	Labels       map[string]uint64
}

type fixtureCallbackInvoker struct {
	invoke func(callbackTypeID uint32, guestID uint64, input []byte) ([]byte, error)
}

func (invoker fixtureCallbackInvoker) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	return invoker.invoke(callbackTypeID, guestID, input)
}

func TestHostConvertsGuestCallbackResourceToTypedGoFunction(t *testing.T) {
	t.Parallel()

	table := resources.NewTable("guest-callback", 8)
	t.Cleanup(func() { require.NoError(t, table.Close()) })
	host := NewHost(table)
	guest := GuestCallback{
		TypeID:  3,
		GuestID: 9,
		Invoker: fixtureCallbackInvoker{invoke: func(
			callbackTypeID uint32,
			guestID uint64,
			input []byte,
		) ([]byte, error) {
			require.EqualValues(t, 3, callbackTypeID)
			require.EqualValues(t, 9, guestID)
			arguments, err := wire.Decode(input)
			require.NoError(t, err)
			require.Equal(t, []any{"gate"}, arguments)
			return wire.EncodeResponse(wire.Response{Values: []any{true}})
		}},
	}
	handle, err := table.Insert(
		guest,
		"fixture.Handler",
		resources.LifetimePlugin,
		nil,
	)
	require.NoError(t, err)

	results, err := host.Call(
		context.Background(),
		Operation{ID: 6, Identity: "fixture.Register"},
		func(handler func(string) (bool, error)) (bool, error) {
			return handler("gate")
		},
		[]any{wire.Resource(handle)},
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []any{true}, results)
}

func TestHostConvertsLanguageNeutralCompositeArguments(t *testing.T) {
	host := NewHost(resources.NewTable("fixture", 2))
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	operation := Operation{ID: 4, Identity: "fixture.Composite"}

	results, err := host.Call(
		context.Background(),
		operation,
		func(input fixtureInput) string {
			return input.HTTPServerID + ":" +
				string(rune(input.Values[1])) + ":" +
				string(rune(input.Labels["answer"]))
		},
		[]any{wire.Record{
			{Name: "http-server-id", Value: "gate"},
			{Name: "values", Value: []any{int64(64), int64(65)}},
			{Name: "labels", Value: wire.Map{
				{Key: "answer", Value: uint64(66)},
			}},
		}},
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []any{"gate:A:B"}, results)
}

func TestHostResolvesResourceArgumentsByGoType(t *testing.T) {
	table := resources.NewTable("fixture", 2)
	host := NewHost(table)
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	receiver := &fixtureReceiver{prefix: "resource:"}
	handle, err := table.Insert(
		receiver,
		"runtime-concrete-type",
		resources.LifetimeOwned,
		nil,
	)
	require.NoError(t, err)

	results, err := host.Call(
		context.Background(),
		Operation{ID: 5, Identity: "fixture.Resource"},
		func(value *fixtureReceiver) string { return value.JoinWithoutError("ok") },
		[]any{wire.Resource(handle)},
		false,
	)
	require.NoError(t, err)
	require.Equal(t, []any{"resource:ok"}, results)
}

func (receiver *fixtureReceiver) JoinWithoutError(value string) string {
	return receiver.prefix + value
}

package wire

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/runtime/resources"
)

func TestValuesRoundTripNestedLanguageNeutralShapes(t *testing.T) {
	t.Parallel()

	input := []any{
		true,
		int64(-42),
		uint64(99),
		float32(1.25),
		math.Pi,
		"héllo 世界",
		[]any{},
		Record{
			{Name: "name", Value: "gate"},
			{Name: "items", Value: []any{uint64(1), nil, "three"}},
		},
		Map{{Key: "answer", Value: int64(42)}},
		Tuple{false, "done"},
		Variant{Name: "present", Value: uint64(7), HasValue: true},
		Enum("ready"),
		Flags{"read", "write"},
		Resource(0x0102030405060708),
		nil,
	}

	encoded, err := Encode(input)
	require.NoError(t, err)
	decoded, err := Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, input, decoded)
}

func TestDecodeRejectsMalformedAndTrailingInput(t *testing.T) {
	t.Parallel()

	_, err := Decode([]byte{Version, 1, tagString, 3, 'x'})
	require.ErrorContains(t, err, "string")

	encoded, err := Encode([]any{"ok"})
	require.NoError(t, err)
	_, err = Decode(append(encoded, 0))
	require.ErrorContains(t, err, "trailing")
}

func TestResponseRoundTrip(t *testing.T) {
	t.Parallel()

	success, err := EncodeResponse(Response{Values: []any{int64(3), "ok"}})
	require.NoError(t, err)
	decodedSuccess, err := DecodeResponse(success)
	require.NoError(t, err)
	require.Equal(t, Response{Values: []any{int64(3), "ok"}}, decodedSuccess)

	failure, err := EncodeResponse(Response{Error: &GateError{
		Kind: "host-error", Message: "bad input", Operation: "pkg.Function",
	}})
	require.NoError(t, err)
	decodedFailure, err := DecodeResponse(failure)
	require.NoError(t, err)
	require.Equal(t, "bad input", decodedFailure.Error.Message)
}

type fixtureRecord struct {
	HTTPServerID string
	Values       []int32
	hidden       string
}

type fixtureResource struct {
	Name string
}

func TestMarshalGoValuesCopiesRecordsAndRegistersResources(t *testing.T) {
	t.Parallel()

	table := resources.NewTable("fixture", 4)
	t.Cleanup(func() { require.NoError(t, table.Close()) })
	resource := &fixtureResource{Name: "proxy"}

	values, err := MarshalGoValues([]any{
		fixtureRecord{
			HTTPServerID: "one",
			Values:       []int32{1, 2},
			hidden:       "not exported",
		},
		resource,
	}, table)
	require.NoError(t, err)
	require.Equal(t, Record{
		{Name: "http-server-id", Value: "one"},
		{Name: "values", Value: []any{int32(1), int32(2)}},
	}, values[0])

	handle := resources.Handle(values[1].(Resource))
	resolved, err := table.Resolve(
		handle,
		"go.minekube.com/gate/internal/wasm/runtime/wire.fixtureResource",
	)
	require.NoError(t, err)
	require.Same(t, resource, resolved)
}

func TestMarshalGoValuesBorrowedExpiresResourcesWithScope(t *testing.T) {
	t.Parallel()

	table := resources.NewTable("callback", 4)
	t.Cleanup(func() { require.NoError(t, table.Close()) })
	scope, err := table.BeginScope(resources.LifetimeBorrowedCall)
	require.NoError(t, err)
	value := &fixtureResource{Name: "event"}

	values, err := MarshalGoValuesBorrowed([]any{value}, table, scope)
	require.NoError(t, err)
	require.Len(t, values, 1)
	handle := resources.Handle(values[0].(Resource))
	resolved, _, err := table.ResolveAny(handle)
	require.NoError(t, err)
	require.Same(t, value, resolved)

	require.NoError(t, scope.Close())
	_, _, err = table.ResolveAny(handle)
	require.ErrorIs(t, err, resources.ErrExpiredHandle)
}

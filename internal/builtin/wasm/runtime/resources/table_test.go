package resources

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTableRejectsForeignAndMistypedHandles(t *testing.T) {
	first := NewTable("first", 4)
	second := NewTable("second", 4)
	t.Cleanup(func() {
		require.NoError(t, first.Close())
		require.NoError(t, second.Close())
	})

	handle, err := first.Insert("value", "fixture.String", LifetimePlugin, nil)
	require.NoError(t, err)
	_, err = second.Resolve(handle, "fixture.String")
	require.ErrorIs(t, err, ErrForeignHandle)
	_, err = first.Resolve(handle, "fixture.Other")
	require.ErrorIs(t, err, ErrTypeMismatch)
	value, err := first.Resolve(handle, "fixture.String")
	require.NoError(t, err)
	require.Equal(t, "value", value)
}

func TestDropReleasesOnceAndReuseInvalidatesStaleHandle(t *testing.T) {
	table := NewTable("plugin", 1)
	var releases atomic.Int32
	first, err := table.Insert(
		"first",
		"fixture.Value",
		LifetimeOwned,
		func() { releases.Add(1) },
	)
	require.NoError(t, err)
	require.NoError(t, table.Drop(first))
	require.EqualValues(t, 1, releases.Load())
	require.ErrorIs(t, table.Drop(first), ErrDoubleDrop)

	second, err := table.Insert(
		"second",
		"fixture.Value",
		LifetimeOwned,
		func() { releases.Add(1) },
	)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	_, err = table.Resolve(first, "fixture.Value")
	require.ErrorIs(t, err, ErrStaleHandle)
	require.NoError(t, table.Close())
	require.EqualValues(t, 2, releases.Load())
	require.NoError(t, table.Close())
	require.EqualValues(t, 2, releases.Load())
}

func TestBorrowedCallAndEventHandlesExpireWithScope(t *testing.T) {
	table := NewTable("plugin", 4)
	t.Cleanup(func() { require.NoError(t, table.Close()) })

	call, err := table.BeginScope(LifetimeBorrowedCall)
	require.NoError(t, err)
	callHandle, err := table.Borrow(
		call,
		"call",
		"fixture.Call",
		nil,
	)
	require.NoError(t, err)
	_, err = table.Resolve(callHandle, "fixture.Call")
	require.NoError(t, err)
	require.NoError(t, call.Close())
	_, err = table.Resolve(callHandle, "fixture.Call")
	require.ErrorIs(t, err, ErrExpiredHandle)
	require.ErrorIs(t, call.Close(), ErrExpiredScope)

	event, err := table.BeginScope(LifetimeBorrowedEvent)
	require.NoError(t, err)
	eventHandle, err := table.Borrow(
		event,
		"event",
		"fixture.Event",
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, event.Close())
	_, err = table.Resolve(eventHandle, "fixture.Event")
	require.ErrorIs(t, err, ErrExpiredHandle)
}

func TestBorrowRejectsWrongOrForeignScope(t *testing.T) {
	first := NewTable("first", 2)
	second := NewTable("second", 2)
	t.Cleanup(func() {
		require.NoError(t, first.Close())
		require.NoError(t, second.Close())
	})

	_, err := first.BeginScope(LifetimeOwned)
	require.ErrorIs(t, err, ErrInvalidLifetime)
	scope, err := first.BeginScope(LifetimeBorrowedCall)
	require.NoError(t, err)
	_, err = second.Borrow(scope, "value", "fixture.Value", nil)
	require.ErrorIs(t, err, ErrForeignScope)
	require.NoError(t, scope.Close())
	_, err = first.Borrow(scope, "value", "fixture.Value", nil)
	require.ErrorIs(t, err, ErrExpiredScope)
}

func TestGateOwnedInvalidationAndCapacity(t *testing.T) {
	table := NewTable("plugin", 1)
	t.Cleanup(func() { require.NoError(t, table.Close()) })
	handle, err := table.Insert(
		"value",
		"fixture.GateOwned",
		LifetimeGateOwned,
		nil,
	)
	require.NoError(t, err)
	_, err = table.Insert("other", "fixture.Other", LifetimePlugin, nil)
	require.ErrorIs(t, err, ErrCapacity)
	require.NoError(t, table.Invalidate(handle))
	_, err = table.Resolve(handle, "fixture.GateOwned")
	require.ErrorIs(t, err, ErrExpiredHandle)
	require.ErrorIs(t, table.Invalidate(handle), ErrExpiredHandle)
}

func TestCloseInvalidatesAllResourcesAndReportsLeaks(t *testing.T) {
	baseline := LiveResources()
	table := NewTable("plugin", 3)
	var releases atomic.Int32
	for index := 0; index < 3; index++ {
		_, err := table.Insert(
			index,
			"fixture.Value",
			LifetimePlugin,
			func() { releases.Add(1) },
		)
		require.NoError(t, err)
	}
	require.EqualValues(t, baseline+3, LiveResources())
	stats := table.Stats()
	require.EqualValues(t, 3, stats.Live)
	require.EqualValues(t, 3, stats.Inserted)

	require.NoError(t, table.Close())
	require.EqualValues(t, 3, releases.Load())
	require.EqualValues(t, baseline, LiveResources())
	stats = table.Stats()
	require.Zero(t, stats.Live)
	require.EqualValues(t, 3, stats.Released)
	_, err := table.Insert("late", "fixture.Value", LifetimePlugin, nil)
	require.ErrorIs(t, err, ErrClosed)
}

func TestStableResourceErrorKinds(t *testing.T) {
	require.True(t, errors.Is(
		&Error{Kind: ErrorTypeMismatch},
		ErrTypeMismatch,
	))
	require.Equal(t, "wasm resource type-mismatch", ErrTypeMismatch.Error())
}

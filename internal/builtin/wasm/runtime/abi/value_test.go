package abi

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestFixedWidthBoundaryLayouts(t *testing.T) {
	require.EqualValues(t, 8, unsafe.Sizeof(Handle(0)))
	require.EqualValues(t, 16, unsafe.Sizeof(BorrowedBuffer{}))
	require.EqualValues(t, 24, unsafe.Sizeof(OwnedBuffer{}))
	require.EqualValues(t, 8, unsafe.Sizeof(OptionHeader{}))
	require.EqualValues(t, 8, unsafe.Sizeof(VariantHeader{}))
	require.EqualValues(t, 16, unsafe.Sizeof(ResultHeader{}))
}

func TestOwnedBuffersNameAllocatorAndFreeOperation(t *testing.T) {
	require.NoError(t, (OwnedBuffer{
		Pointer:  1,
		Length:   4,
		Capacity: 4,
	}).Validate(AllocatorRust))
	require.ErrorContains(t, (OwnedBuffer{
		Length:   1,
		Capacity: 1,
	}).Validate(AllocatorRust), "null pointer")
	require.ErrorContains(t, (OwnedBuffer{
		Pointer:  1,
		Length:   2,
		Capacity: 1,
	}).Validate(AllocatorGo), "length exceeds capacity")
	require.Equal(t, FreeRustBuffer, AllocatorRust.FreeOperation())
	require.Equal(t, FreeGoBuffer, AllocatorGo.FreeOperation())
}

func TestBorrowedBufferLifetimeIsOneCall(t *testing.T) {
	require.Equal(t, LifetimeCall, BorrowedBufferLifetime)
	require.NoError(t, (BorrowedBuffer{Pointer: 1, Length: 1}).Validate())
	require.ErrorContains(
		t,
		(BorrowedBuffer{Length: 1}).Validate(),
		"null pointer",
	)
}

package resources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleRoundTripAndZeroIsInvalid(t *testing.T) {
	handle, err := encodeHandle(23, 7, 42)
	require.NoError(t, err)
	parts, err := DecodeHandle(handle)
	require.NoError(t, err)
	require.EqualValues(t, 23, parts.Owner)
	require.EqualValues(t, 7, parts.Generation)
	require.EqualValues(t, 42, parts.Slot)

	_, err = DecodeHandle(0)
	require.ErrorIs(t, err, ErrInvalidHandle)
	_, err = encodeHandle(0, 1, 1)
	require.ErrorIs(t, err, ErrInvalidHandle)
}

func TestHandleEncodingRejectsZeroParts(t *testing.T) {
	for _, parts := range []HandleParts{
		{Owner: 0, Generation: 1, Slot: 1},
		{Owner: 1, Generation: 0, Slot: 1},
		{Owner: 1, Generation: 1, Slot: 0},
	} {
		_, err := encodeHandle(parts.Owner, parts.Generation, parts.Slot)
		require.ErrorIs(t, err, ErrInvalidHandle)
	}
}

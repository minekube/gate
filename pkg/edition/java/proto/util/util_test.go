package util

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/util/uuid"
	"testing"
)

func TestVarInt(t *testing.T) {
	buf := new(bytes.Buffer)
	require.NoError(t, WriteVarInt(buf, 1))
	i, err := ReadVarInt(buf)
	require.NoError(t, err)
	require.Equal(t, 1, i)
}

func TestUTF(t *testing.T) {
	b := new(bytes.Buffer)
	require.NoError(t, WriteUTF(b, "test"))
	s, err := ReadUTF(b)
	require.NoError(t, err)
	require.Equal(t, "test", s)
}

func TestUUIDIntArray(t *testing.T) {
	// Generate a random UUID
	id := uuid.New()

	// Create a buffer and write the UUID to it as an integer array
	buf := new(bytes.Buffer)
	err := WriteUUIDIntArray(buf, id)
	require.NoError(t, err)

	// Read the UUID from the buffer
	readID, err := ReadUUIDIntArray(buf)
	require.NoError(t, err)

	// The read UUID should be the same as the original UUID
	require.Equal(t, id, readID)
}

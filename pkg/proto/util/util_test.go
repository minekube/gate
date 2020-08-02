package util

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVarInt(t *testing.T) {
	buf := new(bytes.Buffer)
	require.NoError(t, WriteVarInt(buf, 1))
	i, err := ReadVarInt(buf)
	require.NoError(t, err)
	require.Equal(t, 1, i)
}

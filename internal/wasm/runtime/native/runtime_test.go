//go:build !wasm_native || !cgo

package native

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDisabledRuntime(t *testing.T) {
	_, err := New(nil, nil, Limits{})
	require.ErrorIs(t, err, ErrUnavailable)
}

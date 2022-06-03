package randstr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	for i := 0; i < 100; i++ {
		require.Len(t, String(i), i)
	}
}

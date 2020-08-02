package uuid

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOfflinePlayerUuid(t *testing.T) {
	id := OfflinePlayerUuid("bob")
	id2 := OfflinePlayerUuid("bob")
	require.Equal(t, id, id2)

	id2 = OfflinePlayerUuid("Bob")
	require.NotEqual(t, id, id2)
}

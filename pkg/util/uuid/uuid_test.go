package uuid

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOfflinePlayerUUID(t *testing.T) {
	id := OfflinePlayerUUID("bob")
	id2 := OfflinePlayerUUID("bob")
	require.Equal(t, id, id2)

	id2 = OfflinePlayerUUID("Bob")
	require.NotEqual(t, id, id2)
}

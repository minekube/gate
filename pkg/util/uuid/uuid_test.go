package uuid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOfflinePlayerUUID(t *testing.T) {
	id := OfflinePlayerUUID("bob")
	id2 := OfflinePlayerUUID("bob")
	require.Equal(t, id, id2)

	id2 = OfflinePlayerUUID("Bob")
	require.NotEqual(t, id, id2)
}

func TestUUID_JSON(t *testing.T) {
	id := OfflinePlayerUUID("bob")
	b, err := id.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, `"`+id.String()+`"`, string(b))

	var id2 UUID
	err = id2.UnmarshalJSON(b)
	require.NoError(t, err)
	require.Equal(t, id, id2)
}

package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
)

func TestNormalizeDisconnectReasonNil(t *testing.T) {
	reason := normalizeDisconnectReason(nil)

	require.NotNil(t, reason)
	require.IsType(t, &component.Text{}, reason)
}

func TestNormalizeDisconnectReasonTypedNilMarshalsLegacy(t *testing.T) {
	var typedNil *component.Text

	reason := normalizeDisconnectReason(typedNil)

	require.NotNil(t, reason)
	require.IsType(t, &component.Text{}, reason)
	require.NotPanics(t, func() {
		err := (&legacy.Legacy{}).Marshal(new(strings.Builder), reason)
		require.NoError(t, err)
	})
}

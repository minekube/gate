package generate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratedValueSourcesShareCompleteABISchema(t *testing.T) {
	api := simpleAPI()
	schema, err := BuildABI(api)
	require.NoError(t, err)

	goSource, err := RenderGoValues(api)
	require.NoError(t, err)
	rustSource, err := RenderRustValues(api)
	require.NoError(t, err)
	for _, source := range []string{string(goSource), string(rustSource)} {
		require.Contains(t, source, schema.Fingerprint)
		require.Contains(t, source, "ABI schema version")
		require.NotContains(t, source, "uintptr")
		require.NotContains(t, source, "C.long")
	}
	require.Equal(
		t,
		len(schema.Layouts),
		strings.Count(string(goSource), "Identity:"),
	)
	require.Equal(
		t,
		len(schema.Layouts),
		strings.Count(string(rustSource), "    ValueLayout {"),
	)
}

func TestGeneratedValueSourcesAreDeterministic(t *testing.T) {
	api := simpleAPI()
	firstGo, err := RenderGoValues(api)
	require.NoError(t, err)
	firstRust, err := RenderRustValues(api)
	require.NoError(t, err)

	shuffled := cloneAPI(t, api)
	for left, right := 0, len(shuffled.Declarations)-1; left < right; left, right = left+1, right-1 {
		shuffled.Declarations[left], shuffled.Declarations[right] =
			shuffled.Declarations[right], shuffled.Declarations[left]
	}
	secondGo, err := RenderGoValues(shuffled)
	require.NoError(t, err)
	secondRust, err := RenderRustValues(shuffled)
	require.NoError(t, err)
	require.Equal(t, firstGo, secondGo)
	require.Equal(t, firstRust, secondRust)
}

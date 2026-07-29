package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRustDispatchAndCHeaderCoverEveryOperation(t *testing.T) {
	api := simpleAPI()
	wit, err := RenderWIT(api)
	require.NoError(t, err)
	operations, err := Operations(api)
	require.NoError(t, err)
	hash := sha256.Sum256(wit)
	witHash := hex.EncodeToString(hash[:])

	bindings, err := RenderRustBindings(api)
	require.NoError(t, err)
	require.Contains(t, string(bindings), "wasmtime::component::bindgen!")
	require.Contains(t, string(bindings), "world: \"gate-plugin\"")

	dispatch, err := RenderRustDispatch(api)
	require.NoError(t, err)
	require.Contains(t, string(dispatch), witHash)
	require.Equal(
		t,
		len(operations),
		strings.Count(string(dispatch), "    Operation {"),
	)

	header, err := RenderCHeader(api)
	require.NoError(t, err)
	require.Contains(t, string(header), witHash)
	require.Contains(t, string(header), "#define GATE_WASM_OPERATION_COUNT 2")
	require.NotContains(t, string(header), "long")
	require.NotContains(t, string(header), "size_t")
	require.NotContains(t, string(header), "uintptr")
}

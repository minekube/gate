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
	require.Contains(t, string(dispatch), "pub static CALLBACKS: &[CallbackDescriptor]")
	require.Contains(t, string(dispatch), "name: \"handler\"")
	require.Contains(t, string(dispatch), "new_name: \"new-handler\"")
	require.Contains(t, string(dispatch), "call_name: \"call-handler\"")
	require.Contains(
		t,
		string(dispatch),
		"linker.instance(\"minekube:gate/gate-callbacks@0.1.0\")",
	)

	header, err := RenderCHeader(api)
	require.NoError(t, err)
	require.Contains(t, string(header), witHash)
	require.Contains(t, string(header), "#define GATE_WASM_OPERATION_COUNT 2")
	require.NotContains(t, string(header), "long")
	require.NotContains(t, string(header), "size_t")
	require.NotContains(t, string(header), "uintptr")
}

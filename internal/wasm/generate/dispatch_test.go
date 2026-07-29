package generate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/model"
)

func TestOperationsHaveDeterministicContiguousIDs(t *testing.T) {
	api := simpleAPI()
	api.Declarations = append(api.Declarations,
		model.Declaration{
			Identity:    "example.com/project/pkg/shapes.Default",
			PackagePath: "example.com/project/pkg/shapes",
			GoName:      "Default", WITName: "default",
			Kind:     model.DeclarationVariable,
			Type:     typePointer(scalarType("int64", model.TypeS64)),
			Variable: &model.Variable{Readable: true, Writable: true},
			Coverage: model.Coverage{State: model.CoverageRepresented},
		},
		model.Declaration{
			Identity:    "example.com/project/pkg/shapes.Limit",
			PackagePath: "example.com/project/pkg/shapes",
			GoName:      "Limit", WITName: "limit",
			Kind:     model.DeclarationConstant,
			Type:     typePointer(scalarType("int64", model.TypeS64)),
			Constant: &model.Constant{ExactValue: "7"},
			Coverage: model.Coverage{State: model.CoverageRepresented},
		},
	)
	api.Packages[0].Declarations = append(
		api.Packages[0].Declarations,
		"example.com/project/pkg/shapes.Default",
		"example.com/project/pkg/shapes.Limit",
	)
	require.NoError(t, api.Normalize())

	operations, err := Operations(api)
	require.NoError(t, err)
	require.Equal(t, []string{
		"example.com/project/pkg/shapes.Default#get",
		"example.com/project/pkg/shapes.Default#set",
		"example.com/project/pkg/shapes.Limit#get",
		"example.com/project/pkg/shapes.Register",
		"example.com/project/pkg/shapes.Transform",
	}, operationIdentities(operations))
	for index, operation := range operations {
		require.EqualValues(t, index+1, operation.ID)
	}

	shuffled := cloneAPI(t, api)
	for left, right := 0, len(shuffled.Declarations)-1; left < right; left, right = left+1, right-1 {
		shuffled.Declarations[left], shuffled.Declarations[right] =
			shuffled.Declarations[right], shuffled.Declarations[left]
	}
	again, err := Operations(shuffled)
	require.NoError(t, err)
	require.Equal(t, operations, again)
}

func TestRenderGoDispatchBindsEveryOperation(t *testing.T) {
	api := simpleAPI()
	operations, err := Operations(api)
	require.NoError(t, err)
	source, err := RenderGoDispatch(api)
	require.NoError(t, err)
	require.Contains(t, string(source), "func RegisterGeneratedOperations")
	require.Contains(t, string(source), "p000.Register")
	require.Contains(t, string(source), "p000.Transform")
	require.Equal(
		t,
		len(operations),
		strings.Count(string(source), "Handler:"),
	)
	require.NotContains(t, string(source), "recover(")
}

func TestRenderGoCallbacksRegistersBothDirections(t *testing.T) {
	t.Parallel()

	source, err := RenderGoCallbacks(simpleAPI())
	require.NoError(t, err)
	rendered := string(source)
	require.Contains(t, rendered, "var GeneratedCallbacks")
	require.Contains(t, rendered, `Identity:        "example.com/project/pkg/shapes.Handler"`)
	require.Contains(t, rendered, `WITName:         "handler"`)
	require.Contains(t, rendered, "CallOperationID: 2147483649")
	require.Contains(t, rendered, "func RegisterGeneratedCallbacks")
	require.Contains(t, rendered, "host.CallResource")
}

func operationIdentities(operations []GeneratedOperation) []string {
	identities := make([]string, len(operations))
	for index, operation := range operations {
		identities[index] = operation.Identity
	}
	return identities
}

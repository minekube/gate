package analyze

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/model"
)

func TestAnalyzeModelsCallablesVariablesConstantsEventsAndGenerics(t *testing.T) {
	api, err := Analyze(context.Background(), Options{
		Dir:        filepath.Join("testdata", "module"),
		ModulePath: "example.com/fixture",
		Patterns:   []string{"./pkg/shapes"},
	})
	require.NoError(t, err)
	require.Len(t, api.Packages, 1)
	require.NotEmpty(t, api.Declarations)

	for _, declaration := range api.Declarations {
		require.Equal(t, model.CoverageRepresented, declaration.Coverage.State)
		require.NotEmpty(t, declaration.Identity)
		require.NotEmpty(t, declaration.WITName)
		require.NotEmpty(t, declaration.Source.File)
	}

	variadic := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.Variadic",
	)
	require.NotNil(t, variadic.Callable)
	require.True(t, variadic.Callable.Variadic)
	require.Len(t, variadic.Callable.Parameters, 2)
	require.Equal(t, model.TypeList, variadic.Callable.Parameters[1].Type.Kind)
	require.Len(t, variadic.Callable.Results, 1)
	require.NotNil(t, variadic.Callable.Error)

	multiple := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.Multiple",
	)
	require.Len(t, multiple.Callable.Results, 2)

	callback := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.UseCallback",
	)
	require.Equal(t, model.TypeCallback, callback.Callable.Parameters[0].Type.Kind)

	valueMethod := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.Value.Echo",
	)
	require.Equal(t, &model.Receiver{
		TypeIdentity: "example.com/fixture/pkg/shapes.Value",
	}, valueMethod.Receiver)

	pointerMethod := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.Value.SetText",
	)
	require.True(t, pointerMethod.Receiver.Pointer)

	promotedMethod := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.WithEmbedded.Promoted",
	)
	require.True(t, promotedMethod.Receiver.Promoted)

	constant := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.TypedConstant",
	)
	require.Equal(t, "7", constant.Constant.ExactValue)
	require.Equal(t, model.TypeS32, constant.Type.Kind)

	variable := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.WritableValue",
	)
	require.Equal(t, &model.Variable{Readable: true, Writable: true}, variable.Variable)

	event := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.ExampleEvent",
	)
	require.True(t, event.Event)

	generic := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/shapes.Identity[int32]",
	)
	require.NotNil(t, generic.Callable)
	require.Equal(t, model.TypeS32, generic.Callable.Parameters[0].Type.Kind)
	require.Equal(t, model.TypeS32, generic.Callable.Results[0].Type.Kind)
}

func TestAnalyzeRecordsEveryExclusionInCanonicalCoverage(t *testing.T) {
	api, err := Analyze(context.Background(), Options{
		Dir:        filepath.Join("testdata", "module"),
		ModulePath: "example.com/fixture",
		Patterns:   []string{"./pkg/public"},
		Exclusions: []Exclusion{{
			Identity: "example.com/fixture/pkg/public.Excluded",
			Reason:   "fixture exclusion",
		}},
	})
	require.NoError(t, err)

	excluded := findCanonicalDeclaration(
		t, api,
		"example.com/fixture/pkg/public.Excluded",
	)
	require.Equal(t, model.CoverageExcluded, excluded.Coverage.State)
	require.Equal(t, "fixture exclusion", excluded.Coverage.Reason)

	represented := 0
	for _, declaration := range api.Declarations {
		switch declaration.Coverage.State {
		case model.CoverageRepresented:
			represented++
		case model.CoverageExcluded:
		default:
			t.Fatalf(
				"%s has partial coverage state %q",
				declaration.Identity,
				declaration.Coverage.State,
			)
		}
	}
	require.Equal(t, len(api.Declarations)-1, represented)
}

func TestAnalyzeGateRepositoryHasCompleteCoverage(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	api, err := Analyze(context.Background(), GateOptions(root))
	require.NoError(t, err)

	represented := 0
	var excluded []string
	packageDeclarationCount := 0
	for _, pkg := range api.Packages {
		packageDeclarationCount += len(pkg.Declarations)
	}
	for _, declaration := range api.Declarations {
		switch declaration.Coverage.State {
		case model.CoverageRepresented:
			represented++
		case model.CoverageExcluded:
			excluded = append(excluded, declaration.Identity)
		default:
			t.Fatalf(
				"%s has partial coverage state %q",
				declaration.Identity,
				declaration.Coverage.State,
			)
		}
	}
	require.Greater(t, represented, 500)
	require.Equal(t, len(api.Declarations), packageDeclarationCount)
	require.Equal(t, []string{
		"go.minekube.com/gate/pkg/edition/java/proxy.Plugin",
		"go.minekube.com/gate/pkg/edition/java/proxy.Plugins",
	}, excluded)

	subscription := findCanonicalDeclaration(
		t,
		api,
		"go.minekube.com/gate/pkg/edition/java/proxy.ServerPreConnectEvent#wasm-subscribe",
	)
	require.NotNil(t, subscription.Callable)
	require.Len(t, subscription.Callable.Parameters, 2)
	require.Equal(t, model.TypeCallback, subscription.Callable.Parameters[1].Type.Kind)
	require.Equal(
		t,
		model.LifetimeBorrowedEvent,
		subscription.Callable.Parameters[1].Type.Callback.Callable.Parameters[0].Type.Lifetime,
	)
}

func findCanonicalDeclaration(
	t *testing.T,
	api *model.API,
	identity string,
) model.Declaration {
	t.Helper()
	for _, declaration := range api.Declarations {
		if declaration.Identity == identity {
			return declaration
		}
	}
	t.Fatalf("canonical declaration %q not found", identity)
	return model.Declaration{}
}

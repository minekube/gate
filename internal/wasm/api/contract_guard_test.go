package api_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/analyze"
	"go.minekube.com/gate/internal/wasm/generate"
	"go.minekube.com/gate/internal/wasm/model"
)

func TestCommittedContractMatchesCompleteGateAPI(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	current, err := analyze.Analyze(
		context.Background(),
		analyze.GateOptions(root),
	)
	require.NoError(t, err)
	artifacts, err := generate.Artifacts(current)
	require.NoError(t, err)

	for name, expected := range artifacts {
		actual, err := os.ReadFile(name)
		require.NoError(t, err, name)
		require.Equal(t, expected, actual, "%s is stale; run make wasm-api-generate", name)
	}

	var represented, excluded int
	var exclusions []string
	for _, declaration := range current.Declarations {
		switch declaration.Coverage.State {
		case model.CoverageRepresented:
			represented++
		case model.CoverageExcluded:
			excluded++
			exclusions = append(exclusions, declaration.Identity)
		default:
			t.Fatalf(
				"%s has unknown coverage state %q",
				declaration.Identity,
				declaration.Coverage.State,
			)
		}
	}
	require.Equal(t, 2, excluded)
	require.Equal(t, len(current.Declarations), represented+excluded)
	require.Positive(t, represented)
	require.Equal(t, []string{
		"go.minekube.com/gate/pkg/edition/java/proxy.Plugin",
		"go.minekube.com/gate/pkg/edition/java/proxy.Plugins",
	}, exclusions)
}

func TestSignatureMutationChangesHashAndReportsBreakingPath(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	required, err := analyze.Analyze(
		context.Background(),
		analyze.GateOptions(root),
	)
	require.NoError(t, err)
	host := cloneAPI(t, required)

	const commandIdentity = "go.minekube.com/gate/pkg/command.Command"
	command := declaration(t, host, commandIdentity)
	require.NotNil(t, command.Callable)
	require.Len(t, command.Callable.Parameters, 1)
	command.Callable.Parameters[0].Type.Nullable =
		!command.Callable.Parameters[0].Type.Nullable

	requiredWIT, err := generate.RenderWIT(required)
	require.NoError(t, err)
	requiredContract := renderContract(t, required, requiredWIT)
	hostWIT, err := generate.RenderWIT(host)
	require.NoError(t, err)
	hostContract := renderContract(t, host, hostWIT)
	require.NotEqual(t, requiredContract.CanonicalHash, hostContract.CanonicalHash)
	require.NotEqual(t, requiredContract.WITHash, hostContract.WITHash)
	require.EqualError(
		t,
		generate.CheckCompatibility(required, host),
		commandIdentity+".callable.parameters[0].type.nullable: incompatible",
	)
}

func cloneAPI(t *testing.T, source *model.API) *model.API {
	t.Helper()
	encoded, err := json.Marshal(source)
	require.NoError(t, err)
	var cloned model.API
	require.NoError(t, json.Unmarshal(encoded, &cloned))
	return &cloned
}

func declaration(
	t *testing.T,
	api *model.API,
	identity string,
) *model.Declaration {
	t.Helper()
	for index := range api.Declarations {
		if api.Declarations[index].Identity == identity {
			return &api.Declarations[index]
		}
	}
	t.Fatalf("declaration %s is missing", identity)
	return nil
}

func renderContract(
	t *testing.T,
	api *model.API,
	wit []byte,
) generate.Contract {
	t.Helper()
	encoded, err := generate.RenderContract(api, wit)
	require.NoError(t, err)
	var contract generate.Contract
	require.NoError(t, json.Unmarshal(encoded, &contract))
	return contract
}

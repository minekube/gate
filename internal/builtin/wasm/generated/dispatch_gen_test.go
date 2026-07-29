package api

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/codegen/generate"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/dispatch"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/resources"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/util/sets"
)

func TestGeneratedDispatchMatchesManifest(t *testing.T) {
	encoded, err := os.ReadFile(generate.ManifestFile)
	require.NoError(t, err)
	var manifest generate.Manifest
	require.NoError(t, json.Unmarshal(encoded, &manifest))
	contractBytes, err := os.ReadFile(generate.ContractFile)
	require.NoError(t, err)
	var contract generate.Contract
	require.NoError(t, json.Unmarshal(contractBytes, &contract))
	require.Equal(t, contract.WITHash, GeneratedDispatchWITHash)
	require.Equal(t, len(manifest.Operations), len(GeneratedOperations))
	for index, operation := range GeneratedOperations {
		require.EqualValues(t, index+1, operation.ID)
		require.EqualValues(t, manifest.Operations[index].ID, operation.ID)
		require.Equal(t, manifest.Operations[index].Identity, operation.Identity)
	}

	host := dispatch.NewHost(resources.NewTable("fixture", 32))
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	require.NoError(t, RegisterGeneratedOperations(host))

	score := generatedOperation(t, "go.minekube.com/gate/pkg/command/suggest.Score")
	results, err := host.Invoke(
		context.Background(),
		dispatch.OperationID(score.ID),
		[]any{"Gate", "gate"},
	)
	require.NoError(t, err)
	require.Equal(t, []any{float64(0.75)}, results)

	isAndroid := generatedOperation(
		t,
		"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate.DeviceOS.IsAndroid",
	)
	results, err = host.Invoke(
		context.Background(),
		dispatch.OperationID(isAndroid.ID),
		[]any{floodgate.DeviceOSAndroid},
	)
	require.NoError(t, err)
	require.Equal(t, []any{true}, results)
}

func TestGeneratedDispatchInvokesGenericAndResourceMethods(t *testing.T) {
	host := dispatch.NewHost(resources.NewTable("fixture", 32))
	t.Cleanup(func() { require.NoError(t, host.Close()) })
	require.NoError(t, RegisterGeneratedOperations(host))

	constructor := generatedOperation(
		t,
		"go.minekube.com/gate/pkg/util/sets.NewCappedSet[string]",
	)
	results, err := host.Invoke(
		context.Background(),
		dispatch.OperationID(constructor.ID),
		[]any{3},
	)
	require.NoError(t, err)
	set, ok := results[0].(*sets.CappedSet[string])
	require.True(t, ok, "specific generic operation must retain its type arguments")

	handle, err := host.Resources().Insert(
		set,
		"go.minekube.com/gate/pkg/util/sets.CappedSet",
		resources.LifetimePlugin,
		nil,
	)
	require.NoError(t, err)
	add := generatedOperation(
		t,
		"go.minekube.com/gate/pkg/util/sets.CappedSet.Add",
	)
	results, err = host.Invoke(
		context.Background(),
		dispatch.OperationID(add.ID),
		[]any{handle, []string{"one", "two"}},
	)
	require.NoError(t, err)
	require.Empty(t, results)
	require.Equal(t, 2, set.Len())
}

func generatedOperation(
	t *testing.T,
	identity string,
) GeneratedOperationDescriptor {
	t.Helper()
	for _, operation := range GeneratedOperations {
		if operation.Identity == identity {
			return operation
		}
	}
	t.Fatalf("generated operation %s is missing", identity)
	return GeneratedOperationDescriptor{}
}

package api

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/generate"
)

func TestGeneratedValuesMatchContract(t *testing.T) {
	encoded, err := os.ReadFile(generate.ContractFile)
	require.NoError(t, err)
	var contract generate.Contract
	require.NoError(t, json.Unmarshal(encoded, &contract))

	require.Equal(t, contract.ABISchemaVersion, GeneratedABISchemaVersion)
	require.Equal(t, contract.ABILayoutHash, GeneratedABILayoutFingerprint)
	require.NotEmpty(t, GeneratedValueLayouts)
	for _, layout := range GeneratedValueLayouts {
		require.NotEmpty(t, layout.Identity)
		require.Positive(t, layout.Size)
		require.True(t, layout.Alignment != 0 && layout.Alignment&(layout.Alignment-1) == 0)
		require.Zero(t, layout.Size%uint64(layout.Alignment))
		if layout.Allocator != "" {
			require.NotEmpty(t, layout.FreeOperation)
		}
	}
}

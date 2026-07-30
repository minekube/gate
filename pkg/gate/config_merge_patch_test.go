package gate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigJSONUsesCanonicalYAMLFieldNames(t *testing.T) {
	encoded, err := json.Marshal(liveReloadConfig())
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Contains(t, fields, "config")
	require.NotContains(t, fields, "Config")
}

func TestMergeConfigPatchTargetsLiteRoutesAndRejectsUnknownFields(t *testing.T) {
	current := liveReloadConfig()
	candidate, err := mergeConfigPatch(current, `{
		"config": {
			"lite": {
				"routes": [{
					"host": "patched.example.test",
					"backend": "patched-backend.example.test:25565"
				}]
			}
		}
	}`)
	require.NoError(t, err)
	require.Equal(t, current.Config.Bind, candidate.Config.Bind)
	require.Equal(t, []string{"patched.example.test"}, []string(candidate.Config.Lite.Routes[0].Host))
	require.Equal(t, []string{"patched-backend.example.test:25565"}, []string(candidate.Config.Lite.Routes[0].Backend))

	_, err = mergeConfigPatch(current, `{"config":{"unknownOption":true}}`)
	require.Error(t, err)
}

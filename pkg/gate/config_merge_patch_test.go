package gate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
)

func TestConfigJSONUsesCanonicalYAMLFieldNames(t *testing.T) {
	encoded, err := canonicalConfigJSON(liveReloadConfig())
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Contains(t, fields, "config")
	require.NotContains(t, fields, "Config")

	javaConfig := fields["config"].(map[string]any)
	require.Contains(t, javaConfig, "bind")
	require.Contains(t, javaConfig, "status")
	require.Contains(t, javaConfig, "forwarding")
	require.NotContains(t, javaConfig, "Bind")
	require.NotContains(t, javaConfig, "Status")
	require.NotContains(t, javaConfig, "Forwarding")
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

func TestMergeConfigPatchSupportsNestedFieldsAndNullDeletion(t *testing.T) {
	current := liveReloadConfig()
	candidate, err := mergeConfigPatch(current, `{
		"config": {
			"status": {"showMaxPlayers": 42},
			"forwarding": {"mode": "none"}
		}
	}`)
	require.NoError(t, err)
	require.Equal(t, 42, candidate.Config.Status.ShowMaxPlayers)
	require.Equal(t, config.NoneForwardingMode, candidate.Config.Forwarding.Mode)

	candidate, err = mergeConfigPatch(current, `{"config":{"bind":null}}`)
	require.NoError(t, err)
	require.Empty(t, candidate.Config.Bind)
}

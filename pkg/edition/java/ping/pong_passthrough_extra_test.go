package ping

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
)

// TestServerPing_CapturesUnmodeledMembers pins the decode-side contract: the
// members ServerPing does not model are kept in Extra, modeled ones are not.
func TestServerPing_CapturesUnmodeledMembers(t *testing.T) {
	var p ServerPing
	require.NoError(t, json.Unmarshal([]byte(forge1201StatusJSON), &p))
	require.Contains(t, p.Extra, "forgeData")
	require.Len(t, p.Extra, 1)

	var vanilla ServerPing
	require.NoError(t, json.Unmarshal([]byte(`{"description":{"text":"x"},"version":{"protocol":763}}`), &vanilla))
	require.Nil(t, vanilla.Extra)

	// A differently-cased modeled key must not leak into Extra.
	var cased ServerPing
	require.NoError(t, json.Unmarshal([]byte(`{"Description":{"text":"x"},"Version":{"protocol":763}}`), &cased))
	require.Nil(t, cased.Extra)
}

// TestServerPing_ExtraCannotShadowModeledFields keeps the passthrough from
// duplicating or overriding a member Gate does model.
func TestServerPing_ExtraCannotShadowModeledFields(t *testing.T) {
	p := &ServerPing{
		Version:     Version{Protocol: 763, Name: "1.20.1"},
		Players:     &Players{Online: 4, Max: 20},
		Description: &component.Text{Content: "Gate"},
		Extra: map[string]json.RawMessage{
			"version":     json.RawMessage(`{"name":"spoofed","protocol":47}`),
			"description": json.RawMessage(`{"text":"spoofed"}`),
			"forgeData":   json.RawMessage(`{"fmlNetworkVersion":3}`),
			"empty":       json.RawMessage(``),
		},
	}

	encoded, err := json.Marshal(p)
	require.NoError(t, err)

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &doc))
	require.JSONEq(t, `{"protocol":763,"name":"1.20.1"}`, string(doc["version"]))
	require.JSONEq(t, `{"text":"Gate"}`, string(doc["description"]))
	require.JSONEq(t, `{"fmlNetworkVersion":3}`, string(doc["forgeData"]))
	require.NotContains(t, doc, "empty")
}

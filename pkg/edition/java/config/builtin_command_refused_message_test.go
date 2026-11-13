package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/util/configutil"
	"gopkg.in/yaml.v3"
)

// renderedText is what a client receives for a component: the legacy codec
// flattening, colour codes included.
func renderedText(t *testing.T, c component.Component) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, (&legacy.Legacy{}).Marshal(&b, c))
	return b.String()
}

// TestDefaultBuiltinCommandRefusedMessage pins the shipped default: the same
// single red component Gate answered with before the setting existed, with
// {node} as the placeholder (so a configured message uses the same token).
func TestDefaultBuiltinCommandRefusedMessage(t *testing.T) {
	msg := defaultBuiltinCommandRefusedMessage()
	require.NotNil(t, msg)
	require.Equal(t, `You don't have permission to use this command (requires "{node}").`, msg.T().Content)
	require.Equal(t, color.Red, msg.T().S.Color)
	require.Empty(t, msg.T().Extra)

	require.NotNil(t, DefaultConfig.BuiltinCommandRefusedMessage,
		"DefaultConfig must ship the default so the config API exposes it")
	require.Equal(t, msg.T().Content, DefaultConfig.BuiltinCommandRefusedMessage.T().Content)
	require.Equal(t, msg.T().S.Color, DefaultConfig.BuiltinCommandRefusedMessage.T().S.Color)
}

// TestBuiltinCommandRefusedMessageStrictYAML proves the key is reachable
// through the strict decoder the config reload path uses (known fields on), so
// it cannot be silently swallowed as an unknown key.
func TestBuiltinCommandRefusedMessageStrictYAML(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(
		"builtinCommandRefusedMessage: \"§eAsk an admin in #help (needs {node})\"\n"), &node))
	require.NotEmpty(t, node.Content)

	var cfg Config
	require.NoError(t, configutil.DecodeYAMLStrict(node.Content[0], &cfg))

	require.NotNil(t, cfg.BuiltinCommandRefusedMessage)
	require.Equal(t, "§eAsk an admin in #help (needs {node})", renderedText(t, cfg.BuiltinCommandRefusedMessage.T()))
}

// TestBuiltinCommandRefusedMessageJSONRoundTrip proves the key survives the
// JSON marshal Gate's HTTP config API serves and can be read back, with the
// placeholder intact (the substitution happens at render time, per player).
func TestBuiltinCommandRefusedMessageJSONRoundTrip(t *testing.T) {
	cfg := DefaultConfig
	cfg.BuiltinCommandRefusedMessage = &configutil.TextComponent{
		Content: "Ask an admin in #help (needs {node})",
		S:       component.Style{Color: color.Yellow},
	}
	want := renderedText(t, cfg.BuiltinCommandRefusedMessage.T())

	encoded, err := json.Marshal(&cfg)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"builtinCommandRefusedMessage"`)
	require.Contains(t, string(encoded), "{node}")

	var decoded Config
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NotNil(t, decoded.BuiltinCommandRefusedMessage)
	require.Equal(t, want, renderedText(t, decoded.BuiltinCommandRefusedMessage.T()))
}

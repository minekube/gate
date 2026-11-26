package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/util/permission"
	"gopkg.in/yaml.v3"
)

// defaultRefusedMessageRendered is today's hardcoded refusal as the player
// sees it: red, with the required node quoted. The configurable default must
// stay byte-identical to it.
const defaultRefusedMessageRendered = `§cYou don't have permission to use this command (requires "gate.command.server").`

// renderedText is what the client receives: the component flattened by the
// legacy codec, colour codes included.
func renderedText(t *testing.T, c component.Component) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, (&legacy.Legacy{}).Marshal(&b, c))
	return b.String()
}

// refusedMessageConfig parses a config file fragment the same way the real
// loader does (yaml -> configutil.TextComponent, legacy '§' or JSON text).
func refusedMessageConfig(t *testing.T, yamlValue string) *config.Config {
	t.Helper()
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(
		[]byte("builtinCommandRefusedMessage: "+yamlValue+"\n"), &cfg))
	require.NotNil(t, cfg.BuiltinCommandRefusedMessage, "the key must parse from the config file")
	return &cfg
}

// TestBuiltinCmdRefusedMessageDefault pins the fallback: no config (a unit test
// or a nil field) still renders exactly the message Gate answered with before
// the setting existed, in red.
func TestBuiltinCmdRefusedMessageDefault(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *config.Config
	}{
		{name: "nil config", cfg: nil},
		{name: "config without the key", cfg: &config.Config{}},
		{name: "DefaultConfig", cfg: &config.DefaultConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := builtinCmdRefusedMessage(tc.cfg, serverCmdPermission)

			require.Equal(t, `You don't have permission to use this command (requires "gate.command.server").`, msg.Content)
			require.Equal(t, color.Red, msg.S.Color, "the built-in default stays red")
			require.Empty(t, msg.Extra, "the default is the same single component it always was")
			require.Equal(t, defaultRefusedMessageRendered, renderedText(t, msg))
		})
	}
}

// TestBuiltinCmdRefusedMessageConfigured is the load-bearing test for the
// feature: an operator's own refusal text is sent, with every {node} replaced.
func TestBuiltinCmdRefusedMessageConfigured(t *testing.T) {
	t.Run("legacy colour string", func(t *testing.T) {
		cfg := refusedMessageConfig(t, `'§eAsk an admin in #help (needs {node} and {node})'`)

		msg := builtinCmdRefusedMessage(cfg, serverCmdPermission)

		require.Equal(t,
			"§eAsk an admin in #help (needs gate.command.server and gate.command.server)",
			renderedText(t, msg), "every {node} occurrence is substituted")
	})

	t.Run("json text component", func(t *testing.T) {
		cfg := refusedMessageConfig(t, `'{"text":"Ask an admin in #help (needs {node})","color":"yellow"}'`)

		msg := builtinCmdRefusedMessage(cfg, glistCmdPermission)

		require.Equal(t, "§eAsk an admin in #help (needs gate.command.glist)", renderedText(t, msg))
	})

	t.Run("message without the token is sent as-is", func(t *testing.T) {
		cfg := refusedMessageConfig(t, `'§cAsk an admin.'`)

		msg := builtinCmdRefusedMessage(cfg, serverCmdPermission)

		require.Equal(t, "§cAsk an admin.", renderedText(t, msg))
	})

	t.Run("the live config keeps its placeholder", func(t *testing.T) {
		cfg := refusedMessageConfig(t, `'§eAsk an admin (needs {node})'`)

		server := builtinCmdRefusedMessage(cfg, serverCmdPermission)
		send := builtinCmdRefusedMessage(cfg, sendCmdPermission)

		require.Equal(t, "§eAsk an admin (needs gate.command.server)", renderedText(t, server))
		require.Equal(t, "§eAsk an admin (needs gate.command.send)", renderedText(t, send))
		require.Equal(t, "§eAsk an admin (needs {node})", renderedText(t, cfg.BuiltinCommandRefusedMessage.T()),
			"rendering one player's message must not consume the shared config template")
	})
}

// TestBuiltinCommandRefusalUsesConfiguredMessage covers the wiring: the message
// the player actually receives comes from the config, not the hardcoded default.
func TestBuiltinCommandRefusalUsesConfiguredMessage(t *testing.T) {
	cfg := *refusedMessageConfig(t, `'§eAsk an admin in #help (needs {node})'`)
	cfg.BuiltinCommands = true
	cfg.RequireBuiltinCommandPermissions = true

	permFn := func(string) permission.TriState { return permission.Undefined }
	player, conn, cmdMgr, ran := newBuiltinPermFixture(t, &cfg, permFn)

	hasRun, err := executeCommand("server lobby", player, cmdMgr)

	require.NoError(t, err)
	require.True(t, hasRun, "a refused builtin command is consumed, not forwarded")
	require.Empty(t, *ran)
	written := conn.written()
	require.Len(t, written, 1)
	pkt, ok := written[0].(*chat.SystemChat)
	require.True(t, ok, "expected a system chat message, got %T", written[0])
	require.Equal(t, "§eAsk an admin in #help (needs gate.command.server)",
		renderedText(t, pkt.Component.Component))
}

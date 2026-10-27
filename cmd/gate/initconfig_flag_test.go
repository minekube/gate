package gate

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// First-start config generation is opt-in: `gate --init-config` (or
// GATE_INIT_CONFIG=true) writes a config file once, when Gate found none. This
// test pins the flag's existence and its env var, so the documented opt-in
// cannot silently disappear from the CLI (and an unknown flag - which is what
// this test sees on a Gate without the feature - stays a startup error).

func TestInitConfigFlagIsRegistered(t *testing.T) {
	require.NoError(t, App().Run([]string{"gate", "--init-config", "--version"}),
		"--init-config must be a known startup flag")

	var found bool
	for _, f := range App().Flags {
		if f.Names()[0] != "init-config" {
			continue
		}
		found = true
		boolFlag, ok := f.(*cli.BoolFlag)
		require.True(t, ok, "--init-config must be a boolean switch")
		require.Contains(t, boolFlag.EnvVars, "GATE_INIT_CONFIG",
			"--init-config must be settable in a container without CLI arguments")
	}
	require.True(t, found, "the app must declare --init-config")
}

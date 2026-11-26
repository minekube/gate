package gate

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"go.minekube.com/gate/pkg/configs"
)

// `gate config --write` regenerates the shipped template over ./config.yml. A
// config file outlives the command that generated it, so an existing one is an
// operator's work: the command must refuse instead of truncating it (the
// os.WriteFile it used to call clobbered it without a word).

func TestConfigWriteRefusesToOverwriteExistingFile(t *testing.T) {
	t.Chdir(t.TempDir())
	const existing = "config:\n  bind: 127.0.0.1:25566\n"
	require.NoError(t, os.WriteFile("config.yml", []byte(existing), 0o600))

	err, exitCode := runCLI(t, "gate", "config", "--write")
	require.Error(t, err, "writing over an existing config.yml must be refused")
	require.Equal(t, 1, exitCode, "the refusal is a command failure")
	require.True(t, strings.Contains(err.Error(), "already exists"),
		"the refusal must say the file already exists, got %q", err)
	require.True(t, strings.Contains(err.Error(), "--force"),
		"the refusal must name the flag that overwrites deliberately, got %q", err)

	got, readErr := os.ReadFile("config.yml")
	require.NoError(t, readErr)
	require.Equal(t, existing, string(got), "an existing config.yml must never be truncated")
}

func TestConfigWriteForceOverwritesExistingFile(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("config.yml", []byte("config:\n  bind: 127.0.0.1:25566\n"), 0o600))

	err, _ := runCLI(t, "gate", "config", "--write", "--force")
	require.NoError(t, err)

	got, err := os.ReadFile("config.yml")
	require.NoError(t, err)
	require.Equal(t, string(configs.DefaultConfigBytes), string(got),
		"--force must write the selected template, not merge into the old file")
}

func TestConfigWriteCreatesMissingFile(t *testing.T) {
	t.Chdir(t.TempDir())

	err, _ := runCLI(t, "gate", "config", "--write")
	require.NoError(t, err)

	got, err := os.ReadFile("config.yml")
	require.NoError(t, err)
	require.Equal(t, string(configs.DefaultConfigBytes), string(got))
}

// runCLI runs the gate app in-process and reports the error and the exit code
// cli.Exit asked for. urfave/cli hands ExitCoder errors to OsExiter, which is
// os.Exit - that would end the test binary - so the hook is captured instead.
func runCLI(t *testing.T, args ...string) (error, int) {
	t.Helper()
	previous := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = previous })
	exitCode := 0
	cli.OsExiter = func(code int) { exitCode = code }

	err := App().Run(args)
	return err, exitCode
}

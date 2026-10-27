package gate

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"go.minekube.com/gate/pkg/configs"
	"go.minekube.com/gate/pkg/gate"
	"go.minekube.com/gate/pkg/gate/config"
)

// First-start config generation (opt-in, --init-config / GATE_INIT_CONFIG):
// Gate writes a config file once, when it found none, so that an operator who
// never wrote one has a file to edit - and Gate watches it from that boot on.
//
// The properties below are the ones that make the write safe to ship:
//
//	(a) an existing config file is never truncated or overwritten,
//	(b) a failed write is a warning - Gate still boots on the built-in defaults,
//	(c) the generated file parses back to the same effective config as a boot
//	    without any config file, so generating it changes no behaviour,
//	(d) when no target path can be determined at all, the request is a clear
//	    error instead of a silent no-op.
//
// testFirstStartConfig* cover (a), (b), (d); TestGeneratedConfigKeepsBuiltInDefaults
// covers (c).

func TestFirstStartConfigGeneratesFileWhenNoneExists(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	res, err := firstStartConfig("", true)
	require.NoError(t, err)
	require.NoError(t, res.Warn)
	require.False(t, res.Skipped)
	require.Equal(t, filepath.Join(dir, firstStartConfigFileName), res.Written)

	generated, err := os.ReadFile(res.Written)
	require.NoError(t, err)
	require.Equal(t, string(configs.MinimalConfigBytes), string(generated),
		"the generated file must be the behaviour-neutral template")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(res.Written)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o644), info.Mode().Perm(),
			"a generated config is a file the operator edits, not a secret")
	}
}

func TestFirstStartConfigNeverTouchesExistingFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	const existing = "config:\n  bind: 127.0.0.1:25566\n"
	require.NoError(t, os.WriteFile("config.yml", []byte(existing), 0o600))

	res, err := firstStartConfig("", true)
	require.NoError(t, err)
	require.NoError(t, res.Warn)
	require.True(t, res.Skipped, "an existing config.yml must make the write a no-op")
	require.Empty(t, res.Written)

	got, err := os.ReadFile("config.yml")
	require.NoError(t, err)
	require.Equal(t, existing, string(got), "an existing config.yml must never be truncated")
}

func TestFirstStartConfigNeverTouchesExistingExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	const existing = "config:\n  bind: 127.0.0.1:25566\n"
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o600))

	res, err := firstStartConfig(path, true)
	require.NoError(t, err)
	require.True(t, res.Skipped)
	require.Empty(t, res.Written)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, existing, string(got),
		"a config file named with --config must never be truncated either")
}

func TestFirstStartConfigWritesExplicitConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "custom.yaml")
	require.NoError(t, os.Mkdir(filepath.Dir(path), 0o700))

	res, err := firstStartConfig(path, true)
	require.NoError(t, err)
	require.NoError(t, res.Warn)
	require.Equal(t, path, res.Written)
}

// A write that fails is a warning, not a startup failure: the operator asked
// for a file, but Gate's job is to proxy players, and the built-in defaults are
// a working configuration. The parent here is a regular file, so opening the
// target fails for every user - including root in CI.
func TestFirstStartConfigWarnsWhenWriteFails(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	res, err := firstStartConfig(filepath.Join(blocker, "config.yml"), true)
	require.NoError(t, err, "a failed write must not be fatal: Gate boots on built-in defaults")
	require.Error(t, res.Warn, "a failed write must still be reported")
	require.Empty(t, res.Written)
}

// Same property for the default target in an unwritable working directory.
// Root ignores the directory mode, so CI's root containers skip it; the
// ENOTDIR case above covers them.
func TestFirstStartConfigWarnsWhenWorkingDirectoryIsReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a directory mode does not make a directory unwritable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	res, err := firstStartConfig("", true)
	require.NoError(t, err)
	require.Error(t, res.Warn)
	require.Empty(t, res.Written)
}

// A request Gate cannot place is a clear error, not a silent no-op: there is no
// working directory to write ./config.yml into (deleted underneath the
// process).
func TestFirstStartConfigErrorsWhenThereIsNoTargetPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a process cannot remove its own working directory on Windows")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "gone")
	require.NoError(t, os.Mkdir(dir, 0o700))
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))

	res, err := firstStartConfig("", true)
	require.Error(t, err, "no determinable target path must be an error")
	require.Contains(t, err.Error(), "working directory")
	require.Empty(t, res.Written)
}

// Property (c): the generated file is behaviour-neutral. It is compared against
// the config-less boot rather than config.DefaultConfig, because that is the
// configuration Gate actually runs on when no file is found, and switching the
// generated template to an example configuration (full/simple/lite/bedrock)
// fails here.
func TestGeneratedConfigKeepsBuiltInDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// The config-less boot first: same directory, before anything is written.
	resetGlobalViper(t)
	configlessViper, err := initViper(configlessContext(t), "")
	require.NoError(t, err)
	defaultCfg, err := gate.LoadConfig(configlessViper)
	require.Error(t, err, "a config-less boot reports the missing file")
	require.NotNil(t, defaultCfg)

	res, err := firstStartConfig("", true)
	require.NoError(t, err)
	require.NoError(t, res.Warn)
	require.NotEmpty(t, res.Written)

	resetGlobalViper(t)
	generatedViper, err := initViper(explicitConfigContext(t, res.Written), res.Written)
	require.NoError(t, err)
	generatedCfg, err := gate.LoadConfig(generatedViper)
	require.NoError(t, err)

	require.Equal(t, jsonOf(t, defaultCfg), jsonOf(t, generatedCfg),
		"generating a config file must not change any effective setting")
	require.NotEmpty(t, jsonOf(t, &config.DefaultConfig),
		"the built-in defaults must serialize, so the comparison above means something")
}

// The opt-in switch is the only thing that writes: without it a first start
// must stay as quiet as it is today.
func TestFirstStartConfigIsOffByDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	res, err := firstStartConfig("", false)
	require.NoError(t, err)
	require.NoError(t, res.Warn)
	require.Empty(t, res.Written)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "no config file must be generated without --init-config")
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// explicitConfigContext is a cli.Context for the gate app with --config set to
// path - what `gate -c <path>` runs with.
func explicitConfigContext(t *testing.T, path string) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("gate-test", flag.ContinueOnError)
	set.String("config", "", "")
	require.NoError(t, set.Set("config", path))
	return cli.NewContext(App(), set, nil)
}

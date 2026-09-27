package gate

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"go.minekube.com/gate/pkg/gate"
	"go.minekube.com/gate/pkg/gate/config"
)

// Startup config discovery: with no --config, a fresh `gate` process looks for
// ./config.<ext> itself and names the file it found on viper (initViper ->
// findDefaultConfigFile -> SetConfigFile), so that the only read of that file is
// gate.LoadConfig's delete-sharing one (reload.ReadConfigFile). Viper's own read
// uses a handle that shares read and write but not delete, which on Windows
// makes an editor's atomic replacement of config.yml fail with
// ERROR_SHARING_VIOLATION for as long as the read is in flight - Gate refusing
// an operator's config save.
//
// Nothing about the sharing mode is observable on Linux, so the call site is
// pinned (TestDiscoveredConfigFileIsReadByGateNotViper) and the property is
// measured on windows-latest (config_discovery_windows_test.go), the way
// pkg/internal/reload does it. What IS testable everywhere, and lives here, is
// that Gate's resolution picks the same file viper's own discovery would
// (TestDefaultConfigFileDiscoveryMatchesViper) and that a found file is loaded
// and a missing one still reports itself the way `gate` tolerates
// (TestDefaultConfigFileIsLoaded, TestMissingDefaultConfigFileKeepsVipersError).

// TestDefaultConfigFileDiscoveryMatchesViper pins Gate's replication of viper's
// search against viper's own result for a matrix of on-disk layouts: same file
// chosen, same path string (AddConfigPath stores the absolute form, and that is
// what ConfigFileUsed reports), for the discovery settings initViper applies. A
// viper update that changes the order or the layout rules fails here instead of
// silently changing which config Gate loads.
func TestDefaultConfigFileDiscoveryMatchesViper(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		dirs  []string
		want  string
	}{
		{name: "nothing on disk", want: ""},
		{name: "config.yml", files: []string{"config.yml"}, want: "config.yml"},
		{name: "config.yaml wins over config.yml", files: []string{"config.yaml", "config.yml"}, want: "config.yaml"},
		{name: "config.json wins over config.yml", files: []string{"config.yml", "config.json"}, want: "config.json"},
		{name: "config.toml wins over config.yaml", files: []string{"config.yaml", "config.toml"}, want: "config.toml"},
		{name: "unsupported extension only", files: []string{"config.txt"}, want: ""},
		{name: "bare config name only", files: []string{"config"}, want: ""},
		{name: "directory named like a config file", dirs: []string{"config.json"}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), configFileContent(name), 0o600))
			}
			for _, name := range tt.dirs {
				require.NoError(t, os.Mkdir(filepath.Join(dir, name), 0o700))
			}
			t.Chdir(dir)

			// Viper's own discovery for the same settings, as the oracle.
			probe := viper.New()
			probe.SetConfigName(defaultConfigName)
			probe.AddConfigPath(defaultConfigPath)
			err := probe.ReadInConfig()
			require.True(t, err == nil || errors.As(err, &viper.ConfigFileNotFoundError{}),
				"unexpected viper discovery error: %v", err)

			want := ""
			if tt.want != "" {
				want = filepath.Join(dir, tt.want)
			}
			require.Equal(t, want, findDefaultConfigFile(), "Gate must resolve the file viper's discovery would pick")
			require.Equal(t, probe.ConfigFileUsed(), findDefaultConfigFile(),
				"Gate and viper must agree, path string included")
		})
	}
}

// TestDefaultConfigFileIsLoaded checks the behaviour the discovery exists for:
// with no --config, Gate names the config.yml of the working directory on viper
// and loads its values from there.
func TestDefaultConfigFileIsLoaded(t *testing.T) {
	const content = "config:\n  bind: 127.0.0.1:25566\n  lite:\n    enabled: true\n"
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o600))
	t.Chdir(dir)
	resetGlobalViper(t)

	v, err := initViper(configlessContext(t), "")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "config.yml"), v.ConfigFileUsed(),
		"initViper must name the discovered config file on viper")

	cfg, err := gate.LoadConfig(v)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:25566", cfg.Config.Bind, "the discovered config file must be the loaded one")
	require.True(t, cfg.Config.Lite.Enabled, "the discovered config file must be the loaded one")
}

// TestMissingDefaultConfigFileKeepsVipersError pins the other half of the
// contract: with no config file on disk Gate leaves viper to report the miss, so
// the error stays a viper.ConfigFileNotFoundError - the condition cmd/gate
// tolerates (next to os.ErrNotExist) to boot a config-less Gate with defaults
// unless --config was passed.
func TestMissingDefaultConfigFileKeepsVipersError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetGlobalViper(t)

	v, err := initViper(configlessContext(t), "")
	require.NoError(t, err)
	require.Empty(t, v.ConfigFileUsed(), "nothing to name when there is no config file")

	cfg, err := gate.LoadConfig(v)
	require.Error(t, err)
	require.True(t, errors.As(err, &viper.ConfigFileNotFoundError{}),
		"a missing config file must stay a viper.ConfigFileNotFoundError, got %v", err)
	require.NotNil(t, cfg, "the tolerance path in cmd/gate boots the returned default candidate")
	require.Equal(t, config.DefaultConfig.Config.Bind, cfg.Config.Bind,
		"the tolerated error must come with the default config")
}

// TestDiscoveredConfigFileIsReadByGateNotViper pins the call site: the discovery
// branch of initViper resolves the file itself and hands it to viper, and viper
// never reads a config file in cmd/gate. Viper reads through
// afero.ReadFile -> os.ReadFile (read and write shared, delete not), so a read
// on this path is what refuses an editor's atomic config.yml replacement on
// Windows for as long as it is in flight; reverting the resolution would put it
// back. Like pkg/gate's config read contract test, this is the Linux-visible
// guard for a Windows-only property - no source inspection, the shared reader
// and the sharing mode itself are measured by the tests next to the readers.
func TestDiscoveredConfigFileIsReadByGateNotViper(t *testing.T) {
	source, err := os.ReadFile("root.go")
	require.NoError(t, err)
	text := string(source)

	// Strings.Contains over require.Contains/NotContains: a failure message
	// carrying the whole file floods the CI log.
	require.True(t, strings.Contains(text, "if file := findDefaultConfigFile(); file != \"\" {"),
		"initViper must resolve the config file viper's discovery would pick")
	require.True(t, strings.Contains(text, "v.SetConfigFile(file)"),
		"the resolved file must be named on viper, which is what keeps viper from reading it")
	require.False(t, strings.Contains(text, "v.ReadInConfig()"),
		"viper must not read the config file itself: its read handle does not share delete, which on Windows refuses an editor's atomic config.yml save")
}

// configlessContext is a cli.Context for the gate app with no flags set - what
// `gate` runs with when the operator passes no --config and no GATE_CONFIG.
func configlessContext(t *testing.T) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("gate-test", flag.ContinueOnError)
	set.String("config", "", "")
	return cli.NewContext(App(), set, nil)
}

// resetGlobalViper gives the test a fresh gate.Viper, i.e. what a new `gate`
// process starts with (gate.NewViper: viper remembers the config file it was
// pointed at, and sticks to it), and restores the package one afterwards.
func resetGlobalViper(t *testing.T) {
	t.Helper()
	previous := gate.Viper
	t.Cleanup(func() { gate.Viper = previous })
	gate.Viper = gate.NewViper()
}

// configFileContent is parseable content for the file's own format, so that
// viper's oracle read above never fails on the bytes rather than on the search.
func configFileContent(name string) []byte {
	switch filepath.Ext(name) {
	case ".json":
		return []byte("{\"config\": {\"bind\": \"127.0.0.1:25565\"}}")
	case ".toml":
		return []byte("bind = \"127.0.0.1:25565\"\n")
	default:
		return []byte("config:\n  bind: 127.0.0.1:25565\n")
	}
}

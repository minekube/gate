package gate

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfigContentReadsUseTheSharedReader pins gate's config content reads to
// reload.ReadConfigFile. A bare os.ReadFile opens the config with read and write
// sharing but not delete sharing, so on Windows it makes an editor's atomic
// replacement of gate.yml fail with ERROR_SHARING_VIOLATION for as long as the
// read is in flight - Gate itself refusing an operator's config save (#1211
// measured 1208 of 3000 such refusals while os.ReadFile looped over the path).
// Nothing about that is observable on Linux, so the call sites are pinned here.
func TestConfigContentReadsUseTheSharedReader(t *testing.T) {
	source, err := os.ReadFile("gate.go")
	require.NoError(t, err)
	text := string(source)

	require.False(t, strings.Contains(text, "os.ReadFile(configPath)"),
		"config content must be read through reload.ReadConfigFile, not os.ReadFile")
	require.False(t, strings.Contains(text, "os.ReadFile(configFile)"),
		"config content must be read through reload.ReadConfigFile, not os.ReadFile")
	require.Equal(t, 3, strings.Count(text, "reload.ReadConfigFile("),
		"validateConfigFileSyntax, loadLiveConfigCandidate and fixedReadInConfig read the config content")
}

// TestGateViperReadsWithDeleteSharing pins the reads Gate does not make itself:
// the Viper it loads its config with is a NewViper, whose file system serves
// reads with the same delete-sharing handle (reload.ConfigFileSystem), because
// viper reads a file on its own when its search finds none - and Gate must not
// refuse an editor's atomic config save for as long as such a read is in flight
// either (the share mode itself is measured on Windows:
// pkg/internal/reload/config_file_system_windows_test.go, and the loader shape in
// cmd/gate/config_discovery_windows_test.go).
func TestGateViperReadsWithDeleteSharing(t *testing.T) {
	source, err := os.ReadFile("gate.go")
	require.NoError(t, err)
	text := string(source)

	require.True(t, strings.Contains(text, "var Viper = NewViper()"),
		"the Viper Gate loads its config with must be a NewViper")
	require.True(t, strings.Contains(text, "v.SetFs(reload.ConfigFileSystem())"),
		"a NewViper must read config files through reload's delete-sharing file system")
}

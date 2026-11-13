//go:build windows

package reload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfigFileSystemOpenSharesDelete pins the Windows property of
// reload.ConfigFileSystem: a read-only open through it holds a handle that
// shares delete, so an editor's atomic replacement of the file is not refused
// while it is open. Gate reads config content through ReadConfigFile, but a
// library read - viper's ReadInConfig reading the file its own discovery picked
// - goes through this file system, and that read has to share delete too.
//
// The control is deliberate: an unshared handle - what os.Open, i.e. viper's
// default file system, opens - refuses the same replacement.
func TestConfigFileSystemOpenSharesDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	temporary := filepath.Join(dir, "config.yml.tmp")
	require.NoError(t, os.WriteFile(path, []byte("first"), 0o600))
	require.NoError(t, os.WriteFile(temporary, []byte("replacement"), 0o600))

	unshared, err := os.Open(path)
	require.NoError(t, err)
	require.Error(t, replaceFingerprintTestFile(temporary, path),
		"an unshared handle is what makes a read refuse the replacement - the control for the file system below")
	require.NoError(t, unshared.Close())
	require.NoError(t, os.WriteFile(temporary, []byte("replacement"), 0o600))

	shared, err := ConfigFileSystem().Open(path)
	require.NoError(t, err)
	defer shared.Close()

	require.NoError(t, replaceFingerprintTestFile(temporary, path),
		"reading the config through reload's file system must not refuse an editor's atomic replacement")
	content, err := ReadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
}

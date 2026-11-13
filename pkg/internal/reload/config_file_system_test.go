package reload

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// TestConfigFileSystemDelegatesEverythingButReads pins that
// reload.ConfigFileSystem changes read-only opens only: the file system still
// finds, creates, writes, stats and removes files through the OS file system, so
// a library that reads and writes config through viper keeps working while its
// reads share delete (config_file_system_windows_test.go measures the sharing).
func TestConfigFileSystemDelegatesEverythingButReads(t *testing.T) {
	fsys := ConfigFileSystem()
	path := filepath.Join(t.TempDir(), "config.yml")
	content := []byte("config:\n  bind: 127.0.0.1:25565\n")

	require.NoError(t, afero.WriteFile(fsys, path, content, 0o600))
	read, err := afero.ReadFile(fsys, path)
	require.NoError(t, err)
	require.Equal(t, content, read)

	info, err := fsys.Stat(path)
	require.NoError(t, err)
	require.Equal(t, int64(len(content)), info.Size())

	file, err := fsys.Open(path)
	require.NoError(t, err)
	require.Equal(t, path, file.Name(), "a read stays the path it was asked for")
	opened, err := io.ReadAll(file)
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.Equal(t, content, opened)

	require.NoError(t, fsys.Remove(path))
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err), "remove is the OS file system's")
}

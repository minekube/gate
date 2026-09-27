package reload

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadConfigFileReadsContentAndKeepsReadFailureShape pins that the shared
// reader is a drop-in for the os.ReadFile call sites in pkg/gate: same bytes,
// same *os.PathError shape (and the same os.ErrNotExist it unwraps to) when the
// open fails, so read_failed classification and the "\"error reading config
// file\"" wrappers keep working.
func TestReadConfigFileReadsContentAndKeepsReadFailureShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := "config:\n  lite:\n    enabled: true\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	read, err := ReadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(read))

	missing := filepath.Join(dir, "absent.yml")
	_, err = ReadConfigFile(missing)
	require.Error(t, err)
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Equal(t, "open", pathErr.Op)
	require.Equal(t, missing, pathErr.Path)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// TestReadConfigFileBoundsOneRead pins the read bound: an oversized file must
// fail the read instead of being read into memory (and must not be silently
// truncated into a parse error somewhere else).
func TestReadConfigFileBoundsOneRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("config: {}\n"), 0o600))

	require.NoError(t, os.Truncate(path, maxConfigFileBytes+1))

	_, err := ReadConfigFile(path)
	require.Error(t, err)
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Equal(t, "read", pathErr.Op)
	require.Equal(t, path, pathErr.Path)
}

//go:build linux

package gate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A first-start request Gate cannot place anywhere is a clear error, not a
// silent no-op. Linux is the platform where a process can be left in a working
// directory that no longer resolves (rmdir of the cwd makes getcwd fail with
// ENOENT): Darwin's getcwd still names the removed directory, and Windows
// cannot remove a live working directory, so the same scenario is not
// constructible there.
func TestFirstStartConfigErrorsWhenThereIsNoTargetPath(t *testing.T) {
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

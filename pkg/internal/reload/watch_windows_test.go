//go:build windows

package reload

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestReplaceConfigFileRidesOutFingerprintRead pins the Windows behaviour the
// coalescing burst depends on: a fingerprint read in flight makes replacing the
// config fail with "Access is denied" on an otherwise healthy file, so a
// replacement that collides with the watcher's read must retry the transient
// sharing violation until the watcher closes its handle.
//
// The held handle is the one the watcher fingerprints with, and it is held
// across the first attempt so the collision is certain rather than raced: a
// bare os.Rename is refused here and the test fails, which is the CI flake
// TestWatchCoalescesAtomicRenameAndRecreatedFile used to hit (gate#1206's
// windows-latest run 36325999098). Windows that ever allow renaming over an
// open destination would make the retry harmless rather than load-bearing.
func TestReplaceConfigFileRidesOutFingerprintRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	temporary := filepath.Join(dir, "config.yml.tmp")
	require.NoError(t, os.WriteFile(path, []byte("first"), 0o600))
	require.NoError(t, os.WriteFile(temporary, []byte("replacement"), 0o600))

	held, err := openFingerprintFile(path)
	require.NoError(t, err)
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(250 * time.Millisecond)
		_ = held.Close()
	}()
	t.Cleanup(func() { <-released })

	require.NoError(t, replaceConfigFile(temporary, path),
		"replacing a watched config must survive colliding with the watcher's fingerprint read")

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
}

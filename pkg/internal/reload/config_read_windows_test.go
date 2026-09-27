//go:build windows

package reload

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadConfigFileHandleSharesDelete pins the Windows property the loader
// depends on: the handle Gate reads config content with must share delete, or
// Gate refuses an editor's atomic replacement of its own config file for as long
// as the read is in flight (#1211's diagnostic measured 1208 of 3000
// ReplaceFileW replacements refused while os.ReadFile looped over the path).
//
// The control is deliberate: an unshared handle - exactly what os.ReadFile opens
// - refuses the same replacement, so this fails if the reader ever goes back to a
// bare os.ReadFile, and it also fails if a future Windows stopped refusing the
// replacement, which would mean the sharing mode here is no longer observable
// rather than silently fine.
func TestReadConfigFileHandleSharesDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	temporary := filepath.Join(dir, "config.yml.tmp")
	require.NoError(t, os.WriteFile(path, []byte("first"), 0o600))
	require.NoError(t, os.WriteFile(temporary, []byte("replacement"), 0o600))

	unshared, err := os.Open(path)
	require.NoError(t, err)
	require.Error(t, replaceFingerprintTestFile(temporary, path),
		"an unshared handle is what makes Gate refuse the replacement - the control for the shared handle below")
	require.NoError(t, unshared.Close())
	content, err := ReadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, "first", string(content), "a refused replacement must leave the config untouched")

	shared, err := openFingerprintFile(path)
	require.NoError(t, err)
	defer shared.Close()

	require.NoError(t, replaceFingerprintTestFile(temporary, path),
		"reading the config must not refuse an editor's atomic replacement")
	content, err = ReadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
}

// TestReadConfigFileAllowsReplacementsWhileReading is #1211's loader shape,
// re-measured against the production read path: a goroutine loops
// ReadConfigFile(configPath) exactly like Gate's reload path does while
// ReplaceFileW replacements of that path are attempted. Sharing delete on the
// read handle is what keeps every replacement from being refused.
func TestReadConfigFileAllowsReplacementsWhileReading(t *testing.T) {
	const replacements = 1000
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	temporary := filepath.Join(dir, "config.yml.tmp")
	content := "config:\n  lite:\n    enabled: true\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	stop := make(chan struct{})
	readerDone := make(chan struct{})
	var reads atomic.Int64
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			// ReplaceFileW drops the destination name for the duration of the
			// swap, so a read can also observe a transient absence; only the
			// refusals below are this test's subject.
			if _, err := ReadConfigFile(path); err == nil {
				reads.Add(1)
			}
		}
	}()

	refused := 0
	for range replacements {
		require.NoError(t, os.WriteFile(temporary, []byte(content), 0o600))
		if err := replaceFingerprintTestFile(temporary, path); err != nil {
			refused++
		}
	}
	close(stop)
	<-readerDone

	t.Logf("read config %d times, ReplaceFileW refusals %d/%d", reads.Load(), refused, replacements)
	require.Zero(t, refused,
		"Gate's config read must not refuse an operator's atomic config replacement")
	read, err := ReadConfigFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(read))
}

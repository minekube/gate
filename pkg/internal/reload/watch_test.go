package reload

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatchCoalescesAtomicRenameAndRecreatedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("first"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var calls atomic.Int32
	done := make(chan struct{}, 2)
	require.NoError(t, Watch(ctx, path, func() error {
		calls.Add(1)
		done <- struct{}{}
		return nil
	}))

	for range 4 {
		temporary := filepath.Join(dir, "config.yml.tmp")
		require.NoError(t, os.WriteFile(temporary, []byte("replacement"), 0o600))
		require.NoError(t, os.Rename(temporary, path))
	}
	waitWatchCall(t, done)
	time.Sleep(3 * debounceDuration)
	require.Equal(t, int32(1), calls.Load(), "atomic-replace bursts must coalesce")

	require.NoError(t, os.Remove(path))
	time.Sleep(debounceDuration)
	require.NoError(t, os.WriteFile(path, []byte("recreated"), 0o600))
	waitWatchCall(t, done)
}

func TestWatchReportsOnlyRedactedAndRateBoundedRejections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var calls atomic.Int32
	require.NoError(t, Watch(ctx, path, func() error {
		calls.Add(1)
		return Reject("invalid")
	}))
	for range 3 {
		require.NoError(t, os.WriteFile(path, []byte("partial"), 0o600))
	}
	eventually(t, func() bool { return calls.Load() == 1 })
	time.Sleep(3 * debounceDuration)
	require.Equal(t, int32(1), calls.Load())
}

func waitWatchCall(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watched config reload")
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

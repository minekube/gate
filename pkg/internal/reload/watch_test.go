package reload

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
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
		content, err := os.ReadFile(path)
		if err != nil {
			return Reject("read_failed")
		}
		calls.Add(1)
		if string(content) == "replacement" || string(content) == "recreated" {
			done <- struct{}{}
		}
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
	temporary := filepath.Join(dir, "config.yml.recreate")
	require.NoError(t, os.WriteFile(temporary, []byte("recreated"), 0o600))
	require.NoError(t, os.Rename(temporary, path))
	waitWatchCall(t, done)
	require.Equal(t, int32(2), calls.Load())
}

func TestFingerprintReadAllowsAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	temporary := filepath.Join(dir, "config.yml.tmp")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	require.NoError(t, os.WriteFile(temporary, []byte("replacement"), 0o600))

	file, err := openFingerprintFile(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	require.NoError(t, os.Rename(temporary, path))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
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

func TestWatchRedactsAndDeduplicatesArbitraryRejectionCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))

	var (
		logMu sync.Mutex
		logs  []string
	)
	logger := funcr.New(func(prefix, args string) {
		logMu.Lock()
		logs = append(logs, prefix+" "+args)
		logMu.Unlock()
	}, funcr.Options{})
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), logger))
	t.Cleanup(cancel)

	var calls atomic.Int32
	require.NoError(t, Watch(ctx, path, func() error {
		calls.Add(1)
		return Reject("credential=private-endpoint.example")
	}))
	require.NoError(t, os.WriteFile(path, []byte("first"), 0o600))
	eventually(t, func() bool { return calls.Load() == 1 })
	time.Sleep(2 * debounceDuration)
	require.NoError(t, os.WriteFile(path, []byte("second"), 0o600))
	eventually(t, func() bool { return calls.Load() == 2 })
	time.Sleep(2 * debounceDuration)

	logMu.Lock()
	joined := strings.Join(logs, "\n")
	logMu.Unlock()
	require.NotContains(t, joined, "credential")
	require.NotContains(t, joined, "private-endpoint")
	require.Equal(t, 1, strings.Count(joined, "config reload rejected"))
	require.Contains(t, joined, `"reason"="rejected"`)
}

func TestWatchSerializesReloadCallbacks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondDone := make(chan struct{})
	var (
		calls     atomic.Int32
		active    atomic.Int32
		maxActive atomic.Int32
		startOnce sync.Once
	)
	require.NoError(t, Watch(ctx, path, func() error {
		call := calls.Add(1)
		nowActive := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maxActive.Load()
			if nowActive <= previous || maxActive.CompareAndSwap(previous, nowActive) {
				break
			}
		}
		if call == 1 {
			startOnce.Do(func() { close(firstStarted) })
			<-releaseFirst
		}
		if call == 2 {
			close(secondDone)
		}
		return nil
	}))

	require.NoError(t, os.WriteFile(path, []byte("first"), 0o600))
	waitWatchCall(t, firstStarted)
	require.NoError(t, os.WriteFile(path, []byte("second"), 0o600))
	time.Sleep(2 * debounceDuration)
	require.Equal(t, int32(1), calls.Load(), "a second callback must wait for the first")
	close(releaseFirst)
	waitWatchCall(t, secondDone)
	require.Equal(t, int32(1), maxActive.Load())
}

func TestWatchEvaluatesCurrentFileAfterDirectoryReattach(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "config")
	require.NoError(t, os.Mkdir(dir, 0o700))
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	attached := make(chan struct{}, 4)
	reloaded := make(chan string, 4)
	require.NoError(t, watch(ctx, path, func() error {
		content, err := os.ReadFile(path)
		if err != nil {
			return Reject("read_failed")
		}
		reloaded <- string(content)
		return nil
	}, func() { attached <- struct{}{} }))
	waitWatchCall(t, attached)

	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Remove(dir))
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(path, []byte("before-reattach"), 0o600))
	waitWatchCall(t, attached)

	select {
	case content := <-reloaded:
		require.Equal(t, "before-reattach", content)
	case <-time.After(5 * time.Second):
		t.Fatal("current file was not evaluated after watcher reattachment")
	}

	require.NoError(t, os.WriteFile(path, []byte("after-reattach"), 0o600))
	select {
	case content := <-reloaded:
		require.Equal(t, "after-reattach", content)
	case <-time.After(5 * time.Second):
		t.Fatal("write after reattachment did not schedule reload")
	}
}

func TestWatchRecoversFromInvalidThenValidCandidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	valid := make(chan struct{}, 1)
	require.NoError(t, Watch(ctx, path, func() error {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "valid" {
			return Reject("invalid")
		}
		select {
		case valid <- struct{}{}:
		default:
		}
		return nil
	}))

	require.NoError(t, os.WriteFile(path, []byte("partial"), 0o600))
	time.Sleep(2 * debounceDuration)
	require.NoError(t, os.WriteFile(path, []byte("valid"), 0o600))
	waitWatchCall(t, valid)
}

func TestWatchReconcilesChangedRecreatedFileWithoutEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	silent := newSilentEventWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	reloaded := make(chan string, 1)
	require.NoError(t, watchWithOptions(ctx, path, func() error {
		content, err := os.ReadFile(path)
		if err != nil {
			return Reject("read_failed")
		}
		reloaded <- string(content)
		return nil
	}, watchOptions{
		reconcileInterval: 10 * time.Millisecond,
		newWatcher: func(string) (eventWatcher, error) {
			return silent, nil
		},
	}))

	require.NoError(t, os.Remove(path))
	require.NoError(t, os.WriteFile(path, []byte("recreated"), 0o600))
	select {
	case content := <-reloaded:
		require.Equal(t, "recreated", content)
	case <-time.After(5 * time.Second):
		t.Fatal("changed recreated file was not reconciled without fsnotify events")
	}
}

func TestWatchReconciliationIgnoresUnchangedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("unchanged"), 0o600))
	silent := newSilentEventWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var calls atomic.Int32
	require.NoError(t, watchWithOptions(ctx, path, func() error {
		calls.Add(1)
		return nil
	}, watchOptions{
		reconcileInterval: 10 * time.Millisecond,
		newWatcher: func(string) (eventWatcher, error) {
			return silent, nil
		},
	}))

	require.NoError(t, os.WriteFile(path, []byte("unchanged"), 0o600))
	time.Sleep(2*debounceDuration + 50*time.Millisecond)
	require.Zero(t, calls.Load())
}

func TestWatchCancellationClosesSilentWatcherAndStopsReconciliation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))
	silent := newSilentEventWatcher()
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	require.NoError(t, watchWithOptions(ctx, path, func() error {
		calls.Add(1)
		return nil
	}, watchOptions{
		reconcileInterval: 10 * time.Millisecond,
		newWatcher: func(string) (eventWatcher, error) {
			return silent, nil
		},
	}))

	cancel()
	waitWatchCall(t, silent.closed)
	require.NoError(t, os.WriteFile(path, []byte("changed-after-cancel"), 0o600))
	time.Sleep(2*debounceDuration + 50*time.Millisecond)
	require.Zero(t, calls.Load())
}

type silentEventWatcher struct {
	events    chan fsnotify.Event
	errors    chan error
	closed    chan struct{}
	closeOnce sync.Once
}

func newSilentEventWatcher() *silentEventWatcher {
	return &silentEventWatcher{
		events: make(chan fsnotify.Event),
		errors: make(chan error),
		closed: make(chan struct{}),
	}
}

func (w *silentEventWatcher) Events() <-chan fsnotify.Event { return w.events }
func (w *silentEventWatcher) Errors() <-chan error          { return w.errors }
func (w *silentEventWatcher) Close() error {
	w.closeOnce.Do(func() { close(w.closed) })
	return nil
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

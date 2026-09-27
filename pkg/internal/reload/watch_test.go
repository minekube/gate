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

	// The burst has to be a replacement that keeps the destination present at
	// every instant - what rename means on POSIX and what MoveFileEx with
	// MOVEFILE_REPLACE_EXISTING means on Windows. Windows refuses that
	// replacement while the watcher is holding the destination open to
	// fingerprint it, so a bare os.Rename here races the watcher's read instead
	// of testing it (measured on windows-latest: 1319/3000 iterations failed
	// with "Access is denied" with the watcher running, 0/3000 with it
	// stopped). replaceWatchedConfig rides out that transient sharing
	// violation, as an editor replacing a watched config must, and never leaves
	// the destination in a state the watcher could read as a config.
	for range 4 {
		replaceWatchedConfig(t, path, "replacement")
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

	require.NoError(t, replaceFingerprintTestFile(temporary, path))
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
	// The burst has to replace the config, not write it in place. The watcher
	// reports every content state it can observe, and an in-place write
	// (os.WriteFile: O_TRUNC, then the bytes) exposes the destination's
	// truncated state as one of them. On a loaded runner the writer is
	// descheduled between those two syscalls, the watcher reconciles the empty
	// destination and its debounce elapses before the bytes land, so the settled
	// content is reported as a second change - two reloads where this burst must
	// coalesce into one, which is the ubuntu-latest flake of run 36328594414
	// ("expected: 1 actual: 2"). Replacing the file keeps the destination
	// present at every instant, so the burst has one state to report however the
	// runner schedules the writer;
	// TestWatchCoalescesIdenticalWritesAcrossADebounceWindow forces that
	// interleaving and pins the claim on it.
	for range 3 {
		replaceWatchedConfig(t, path, "partial")
	}
	eventually(t, func() bool { return calls.Load() == 1 })
	time.Sleep(3 * debounceDuration)
	require.Equal(t, int32(1), calls.Load())
}

// TestWatchCoalescesIdenticalWritesAcrossADebounceWindow forces the interleaving
// the coalescing fixtures have to survive instead of racing it: the burst's
// later writes land after the watcher's debounce for the first one has already
// elapsed, which is the state a loaded runner produces by descheduling the
// writer mid-write (ubuntu-latest run 36328594414). The watcher is driven by the
// events this test hands it and every step is ordered by a handoff, so the
// interleaving is certain rather than probed.
//
// The burst replaces the config (replaceWatchedConfig), which is what makes the
// claim true: an in-place write of the same content instead exposes the
// destination's truncated state, the watcher reports it as a change, and this
// assertion fails with the CI's signature - the fixture's write has to keep the
// destination present at every instant.
func TestWatchCoalescesIdenticalWritesAcrossADebounceWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("initial"), 0o600))

	silent := newSilentEventWatcher()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var calls atomic.Int32
	reloads := make(chan struct{}, 4)
	require.NoError(t, watchWithOptions(ctx, path, func() error {
		calls.Add(1)
		reloads <- struct{}{}
		return Reject("invalid")
	}, watchOptions{
		// This test owns every reconcile: the loop runs on the events it is
		// handed (and the debounce they schedule), not on a ticker.
		reconcileInterval: time.Hour,
		newWatcher: func(string) (eventWatcher, error) {
			return silent, nil
		},
	}))

	// The first write is reported before the burst continues: that straddle is
	// the whole flake, and the watch loop's callback is its edge.
	replaceWatchedConfig(t, path, "partial")
	silent.sendEvent(t, fsnotify.Event{Name: path, Op: fsnotify.Write})
	waitWatchCall(t, reloads)

	// The remaining two writes replace the same content, so there is nothing new
	// to report - that is the coalescing claim.
	for range 2 {
		replaceWatchedConfig(t, path, "partial")
		silent.sendEvent(t, fsnotify.Event{Name: path, Op: fsnotify.Write})
	}

	// A reload the burst scheduled would fire within one debounce window of the
	// reconcile that scheduled it, so waiting longer than that proves the burst
	// scheduled nothing rather than winning a race against the assertion.
	select {
	case <-reloads:
	case <-time.After(3 * debounceDuration):
	}
	require.Equal(t, int32(1), calls.Load(), "three identical writes must coalesce into one reload")

	// The loop is alive and still reports real changes, so the assertion above
	// cannot pass because the watch loop stopped.
	replaceWatchedConfig(t, path, "settled")
	silent.sendEvent(t, fsnotify.Event{Name: path, Op: fsnotify.Write})
	waitWatchCall(t, reloads)
	require.Equal(t, int32(2), calls.Load())
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

// sendEvent hands the watch loop one filesystem event and returns once the loop
// has taken it, which is the moment it reconciles the config. Tests that order
// themselves on these handoffs force an interleaving instead of sleeping for it.
func (w *silentEventWatcher) sendEvent(t *testing.T, event fsnotify.Event) {
	t.Helper()
	select {
	case w.events <- event:
	case <-w.closed:
		t.Fatal("watch loop closed before it took the event")
	case <-time.After(5 * time.Second):
		t.Fatal("watch loop did not take the event")
	}
}

// replaceWatchedConfig replaces the watched config at path with content the way
// a config editor does: a temporary file next to it, then an atomic replacement
// that keeps the destination present at every instant. Fixtures that count
// reloads have to write this way - an in-place write exposes the destination's
// truncated state, which the watcher correctly reports as a change of its own.
func replaceWatchedConfig(t *testing.T, path, content string) {
	t.Helper()
	temporary := filepath.Join(filepath.Dir(path), filepath.Base(path)+".tmp")
	require.NoError(t, os.WriteFile(temporary, []byte(content), 0o600))
	require.NoError(t, replaceConfigFile(temporary, path))
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

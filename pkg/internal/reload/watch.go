package reload

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

const debounceDuration = 100 * time.Millisecond
const reconciliationInterval = 250 * time.Millisecond
const rejectionLogInterval = time.Minute

// rejection is deliberately a short category. It prevents reload diagnostics
// from exposing raw configuration, endpoints, or credentials.
type rejection struct{ code string }

func (e rejection) Error() string { return e.code }

// Reject returns an error suitable for a reload callback when a candidate was
// rejected. Arbitrary values are reduced to a non-sensitive category.
func Reject(code string) error { return rejection{code: sanitizeRejectionCode(code)} }

func sanitizeRejectionCode(code string) string {
	switch code {
	case "read_failed", "parse_failed", "invalid", "unsupported", "prepare_failed":
		return code
	default:
		return "rejected"
	}
}

type eventWatcher interface {
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

type fsnotifyEventWatcher struct{ watcher *fsnotify.Watcher }

func (w *fsnotifyEventWatcher) Events() <-chan fsnotify.Event { return w.watcher.Events }
func (w *fsnotifyEventWatcher) Errors() <-chan error          { return w.watcher.Errors }
func (w *fsnotifyEventWatcher) Close() error                  { return w.watcher.Close() }

type watchOptions struct {
	reconcileInterval time.Duration
	newWatcher        func(string) (eventWatcher, error)
	attached          func()
}

// Watch monitors the active configuration file. Filesystem notifications are
// low-latency hints; periodic content fingerprints reconcile events that an OS
// watcher may coalesce or miss. One goroutine owns all watcher, timer, callback,
// and diagnostic state.
func Watch(ctx context.Context, path string, cb func() error) error {
	return watchWithOptions(ctx, path, cb, watchOptions{})
}

func watch(ctx context.Context, path string, cb func() error, attached func()) error {
	return watchWithOptions(ctx, path, cb, watchOptions{attached: attached})
}

func watchWithOptions(ctx context.Context, configPath string, cb func() error, opts watchOptions) error {
	if ctx.Err() != nil {
		return nil
	}
	if opts.reconcileInterval <= 0 {
		opts.reconcileInterval = reconciliationInterval
	}
	if opts.newWatcher == nil {
		opts.newWatcher = newDirectoryWatcher
	}

	cleanPath := filepath.Clean(configPath)
	dir := filepath.Dir(cleanPath)
	name := filepath.Base(cleanPath)
	watcher, err := opts.newWatcher(dir)
	if err != nil {
		return err
	}
	if opts.attached != nil {
		opts.attached()
	}

	initial := fingerprint(cleanPath)
	go runWatchLoop(ctx, cleanPath, dir, name, initial, watcher, cb, opts)
	return nil
}

func newDirectoryWatcher(dir string) (eventWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(dir)
	if parent != dir {
		if err := watcher.Add(parent); err != nil {
			_ = watcher.Close()
			return nil, err
		}
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	return &fsnotifyEventWatcher{watcher: watcher}, nil
}

type contentFingerprint struct {
	state byte
	sum   [sha256.Size]byte
}

func fingerprint(path string) contentFingerprint {
	content, err := os.ReadFile(path)
	if err == nil {
		return contentFingerprint{state: 1, sum: sha256.Sum256(content)}
	}
	if errors.Is(err, os.ErrNotExist) {
		return contentFingerprint{state: 2}
	}
	return contentFingerprint{state: 3}
}

func runWatchLoop(
	ctx context.Context,
	configPath, dir, name string,
	evaluated contentFingerprint,
	watcher eventWatcher,
	cb func() error,
	opts watchOptions,
) {
	log := logr.FromContextOrDiscard(ctx).WithName("config-reload")
	observed := evaluated

	reconcileTicker := time.NewTicker(opts.reconcileInterval)
	defer reconcileTicker.Stop()
	debounceTimer := time.NewTimer(time.Hour)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}
	defer debounceTimer.Stop()
	var debounce <-chan time.Time

	stopDebounce := func() {
		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounce = nil
	}
	schedule := func() {
		stopDebounce()
		debounceTimer.Reset(debounceDuration)
		debounce = debounceTimer.C
	}
	reconcile := func() {
		current := fingerprint(configPath)
		if current == observed {
			return
		}
		observed = current
		schedule()
	}

	var (
		events        <-chan fsnotify.Event
		watcherErrors <-chan error
	)
	bindWatcher := func() {
		if watcher == nil {
			events = nil
			watcherErrors = nil
			return
		}
		events = watcher.Events()
		watcherErrors = watcher.Errors()
	}
	closeWatcher := func() {
		if watcher != nil {
			_ = watcher.Close()
			watcher = nil
		}
		bindWatcher()
	}
	defer closeWatcher()
	bindWatcher()

	var (
		lastRejection string
		lastLoggedAt  time.Time
	)
	runCallback := func(candidate contentFingerprint) {
		if candidate == evaluated {
			return
		}
		evaluated = candidate
		start := time.Now()
		if err := cb(); err != nil {
			code := "read_failed"
			var rejected rejection
			if errors.As(err, &rejected) {
				code = sanitizeRejectionCode(rejected.code)
			}
			now := time.Now()
			if code != lastRejection || now.Sub(lastLoggedAt) >= rejectionLogInterval {
				lastRejection, lastLoggedAt = code, now
				log.Info("config reload rejected", "reason", code)
			}
			return
		}
		lastRejection = ""
		lastLoggedAt = time.Time{}
		log.Info("config reloaded", "duration", time.Since(start).Round(time.Millisecond).String())
	}

	for {
		select {
		case <-ctx.Done():
			stopDebounce()
			return
		case <-reconcileTicker.C:
			if watcher == nil {
				next, err := opts.newWatcher(dir)
				if err == nil {
					watcher = next
					bindWatcher()
					if opts.attached != nil {
						opts.attached()
					}
				}
			}
			reconcile()
		case <-debounce:
			debounce = nil
			runCallback(observed)
		case event, ok := <-events:
			if !ok {
				closeWatcher()
				continue
			}
			eventPath := filepath.Clean(event.Name)
			if eventPath == dir && event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				closeWatcher()
				continue
			}
			if filepath.Dir(eventPath) == dir && filepath.Base(eventPath) == name {
				reconcile()
			}
		case _, ok := <-watcherErrors:
			if !ok {
				closeWatcher()
				continue
			}
			closeWatcher()
		}
	}
}

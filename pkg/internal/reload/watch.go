package reload

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

const debounceDuration = 100 * time.Millisecond
const rejectionLogInterval = time.Minute

// rejection is deliberately a short category. It prevents reload diagnostics
// from exposing raw configuration, endpoints, or credentials.
type rejection struct{ code string }

func (e rejection) Error() string { return e.code }

// Reject returns an error suitable for a reload callback when a candidate was
// rejected. Code must be a stable, non-sensitive category.
func Reject(code string) error { return rejection{code: code} }

// Watch monitors the parent directory so atomic rename and delete/recreate
// writes remain observable. Each Watch has independent debounce state; a lost
// fsnotify watcher is recreated until its context ends.
func Watch(ctx context.Context, path string, cb func() error) error {
	if ctx.Err() != nil {
		return nil
	}
	dir, name := filepath.Dir(filepath.Clean(path)), filepath.Base(path)
	watcher, err := newDirectoryWatcher(dir)
	if err != nil {
		return err
	}

	go func() {
		for {
			watchLoop(ctx, dir, name, watcher, cb)
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(debounceDuration):
			}
			var err error
			watcher, err = newDirectoryWatcher(dir)
			if err != nil {
				// Keep retrying at a deterministic interval. There is no callback
				// because a watcher loss is not a candidate configuration change.
				select {
				case <-ctx.Done():
					return
				case <-time.After(debounceDuration):
				}
				continue
			}
		}
	}()
	return nil
}

func newDirectoryWatcher(dir string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	return watcher, nil
}

func watchLoop(ctx context.Context, dir, name string, watcher *fsnotify.Watcher, cb func() error) {
	log := logr.FromContextOrDiscard(ctx).WithName("config-reload")
	defer watcher.Close()

	var (
		mu            sync.Mutex
		timer         *time.Timer
		lastRejection string
		lastLoggedAt  time.Time
	)
	defer func() {
		mu.Lock()
		if timer != nil {
			timer.Stop()
		}
		mu.Unlock()
	}()

	run := func() {
		start := time.Now()
		if err := cb(); err != nil {
			code := "read_failed"
			var rejected rejection
			if errors.As(err, &rejected) {
				code = rejected.code
			}
			now := time.Now()
			mu.Lock()
			shouldLog := code != lastRejection || now.Sub(lastLoggedAt) >= rejectionLogInterval
			if shouldLog {
				lastRejection, lastLoggedAt = code, now
			}
			mu.Unlock()
			if shouldLog {
				log.Info("config reload rejected", "reason", code)
			}
			return
		}
		mu.Lock()
		lastRejection = ""
		lastLoggedAt = time.Time{}
		mu.Unlock()
		log.Info("config reloaded", "duration", time.Since(start).Round(time.Millisecond).String())
	}

	schedule := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(debounceDuration, run)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Dir(filepath.Clean(event.Name)) == dir && filepath.Base(event.Name) == name {
				schedule()
			}
		case <-watcher.Errors:
			// A directory watch normally survives a file replacement. If the OS
			// closes it, returning lets the supervising process establish a fresh
			// watcher on its next configuration event/start cycle.
			return
		}
	}
}

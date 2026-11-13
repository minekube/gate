// Package logfile writes Gate's log to a file on disk so the log survives a
// process restart.
//
// Gate logs to stderr, which is the right default for containers and services
// that collect the stream themselves. When nothing collects it, a restart takes
// the whole log with it. This writer is the opt-in alternative: the logger tees
// into it, and the file is appended to rather than truncated, so a restarted
// proxy keeps the previous run's lines.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FileName is the active log file inside the configured directory.
const FileName = "gate.log"

// maxBackupGlob matches rotated files, e.g. gate.2026-10-26T12-34-56.789.log.
const maxBackupGlob = "gate.*.log"

// Writer appends to <dir>/gate.log and rotates the file once it grows past a
// size limit. It is safe for concurrent use, which zap requires.
//
// Writes go straight to the file (no buffering), so a crash cannot lose lines
// that were already logged.
type Writer struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	size int64
	// maxSize is the rotation threshold in bytes; 0 disables rotation.
	maxSize int64
	// maxBackups is how many rotated files to keep; 0 keeps every one of them.
	maxBackups int
	closed     bool
}

// Open creates dir if needed and returns a writer appending to <dir>/gate.log.
//
// maxSizeMB <= 0 disables rotation. maxBackups is the number of rotated files to
// keep, oldest deleted first; 0 keeps them all (rotation without deletion).
func Open(dir string, maxSizeMB, maxBackups int) (*Writer, error) {
	if dir == "" {
		return nil, fmt.Errorf("log directory is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory %q: %w", dir, err)
	}
	w := &Writer{
		dir:        dir,
		maxSize:    int64(maxSizeMB) * 1024 * 1024,
		maxBackups: maxBackups,
	}
	if w.maxSize < 0 {
		w.maxSize = 0
	}
	if w.maxBackups < 0 {
		w.maxBackups = 0
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	path := filepath.Join(w.dir, FileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log file %q: %w", path, err)
	}
	w.file = f
	w.size = info.Size()
	return nil
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, fmt.Errorf("log file %q is closed", filepath.Join(w.dir, FileName))
	}
	if w.maxSize > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Sync flushes the file to stable storage; it exists for completeness, writes
// are already unbuffered.
func (w *Writer) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

// Close closes the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.file == nil {
		w.closed = true
		return nil
	}
	w.closed = true
	err := w.file.Close()
	w.file = nil
	return err
}

// rotate moves the active file aside and starts a new one.
func (w *Writer) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("close log file for rotation: %w", err)
		}
		w.file = nil
	}
	active := filepath.Join(w.dir, FileName)
	// Millisecond precision: a proxy that is misconfigured (limit far too small)
	// can rotate several times within one second and must not overwrite its own
	// backups.
	stamp := time.Now().UTC().Format("2006-01-02T15-04-05.000")
	backup := filepath.Join(w.dir, "gate."+stamp+".log")
	if err := os.Rename(active, backup); err != nil && !os.IsNotExist(err) {
		// Reopening the active file below keeps logging alive even if the rename
		// failed; report it so the operator sees the problem once.
		if err := w.open(); err != nil {
			return err
		}
		return fmt.Errorf("rotate log file %q: %w", active, err)
	}
	if err := w.prune(); err != nil {
		// Pruning is housekeeping: a failure must not stop logging.
		if err := w.open(); err != nil {
			return err
		}
		return fmt.Errorf("prune rotated logs: %w", err)
	}
	return w.open()
}

// prune deletes the oldest rotated files beyond maxBackups.
//
// maxBackups 0 means "keep everything": rotation still bounds the size of the
// active file, and the operator decides when to delete history.
func (w *Writer) prune() error {
	if w.maxBackups <= 0 {
		return nil
	}
	backups, err := filepath.Glob(filepath.Join(w.dir, maxBackupGlob))
	if err != nil || len(backups) <= w.maxBackups {
		return err
	}
	// Timestamped names sort chronologically, oldest first.
	sort.Strings(backups)
	for _, old := range backups[:len(backups)-w.maxBackups] {
		if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

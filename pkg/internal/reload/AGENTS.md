# pkg/internal/reload agent notes

Config reload correctness must not rely on fsnotify delivery alone: `pkg/internal/reload/watch.go` reconciles content fingerprints because macOS can miss delete/recreate events. On Windows, fingerprint handles must allow delete sharing or they can make an editor's atomic replacement fail with access denied. Go's `os.Rename`/`MoveFileExW` still cannot replace an open destination, so open-handle replacement coverage uses `ReplaceFileW`. Preserve its single-owner loop and silent-watcher regression coverage when changing debounce or watcher behavior.

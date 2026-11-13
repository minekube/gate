//go:build windows

package reload

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// replaceRetryWindow bounds how long a replacement rides out the watcher's
// reads. A fingerprint read lasts microseconds, so this is thousands of
// attempts of headroom; a watcher that holds the config open longer than this
// is a leaked handle and must fail the test rather than be waited out.
const replaceRetryWindow = 5 * time.Second

// replaceConfigFile replaces the watched config file with source, the way an
// editor saving a config does on Windows.
//
// Windows refuses to replace a destination that any process holds open, no
// matter the handle's share mode: MoveFileEx(MOVEFILE_REPLACE_EXISTING) - which
// is what os.Rename calls - returns "Access is denied" while the reload watcher
// is reading the config to fingerprint it. Measured on windows-latest: of 3000
// write+rename iterations, 1319 failed with the watcher running and 0 failed
// with it stopped, so the collision is the watcher's read and not the fixture;
// no share mode (read, write, delete, none) makes os.Rename replace an open
// destination, and the watcher cannot hash the file without opening it.
//
// The remaining Windows-valid atomic replacement is therefore "retry the
// transient sharing violation", which is what editors and installers do.
// ReplaceFileW is not a substitute for a watched file: it removes the
// destination name for the duration of the swap, so the watcher reports the
// gap as a second change (two reloads where the burst must coalesce into one).
func replaceConfigFile(source, destination string) error {
	deadline := time.Now().Add(replaceRetryWindow)
	for {
		err := os.Rename(source, destination)
		if err == nil || time.Now().After(deadline) || !isConfigSharingViolation(err) {
			return err
		}
		time.Sleep(time.Millisecond)
	}
}

// Windows sharing refusals that mean "another handle has this file open", not
// "the fixture is broken": syscall only exports ERROR_ACCESS_DENIED.
const (
	errorSharingViolation syscall.Errno = 32 // ERROR_SHARING_VIOLATION
	errorLockViolation    syscall.Errno = 33 // ERROR_LOCK_VIOLATION
)

// isConfigSharingViolation reports whether err is a transient Windows refusal
// to touch a file another handle has open, not a real fixture error.
func isConfigSharingViolation(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) ||
		errors.Is(err, errorSharingViolation) ||
		errors.Is(err, errorLockViolation)
}

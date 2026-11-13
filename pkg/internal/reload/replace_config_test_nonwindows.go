//go:build !windows

package reload

import "os"

// replaceConfigFile replaces the watched config file with source, the way an
// editor saving a config does. POSIX rename replaces a destination that other
// processes hold open, so no retry is needed here.
func replaceConfigFile(source, destination string) error {
	return os.Rename(source, destination)
}

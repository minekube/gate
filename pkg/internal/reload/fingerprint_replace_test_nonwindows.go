//go:build !windows

package reload

import "os"

func replaceFingerprintTestFile(source, destination string) error {
	return os.Rename(source, destination)
}

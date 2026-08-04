//go:build !windows

package reload

import "os"

func openFingerprintFile(path string) (*os.File, error) {
	return os.Open(path)
}

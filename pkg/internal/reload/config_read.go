package reload

import (
	"errors"
	"io"
	"os"
)

// maxConfigFileBytes bounds one config content read. Gate configs are a few
// KiB, so this only ever rejects a pathological file - or one another process
// keeps appending to - instead of pulling an unbounded amount of it into
// memory.
const maxConfigFileBytes = 8 << 20

// ReadConfigFile reads one complete config file image for a loader.
//
// It opens the file through openFingerprintFile, i.e. the same handle the
// watcher fingerprints with, because that is the handle Gate needs here too: on
// Windows a read whose handle shares read and write but not delete makes an
// editor's atomic replacement of the config ("ReplaceFileW", the API Windows
// provides for replacing a file another process has open) fail with
// ERROR_SHARING_VIOLATION for as long as the read is in flight, so Gate would
// be the process refusing an operator's config save. Nothing else about the
// read changes: the open error is returned as-is and a failed read is wrapped
// in the same *os.PathError os.ReadFile produces, so callers keep classifying
// read failures the way they do today.
func ReadConfigFile(path string) ([]byte, error) {
	file, err := openFingerprintFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxConfigFileBytes+1))
	if err != nil {
		return nil, &os.PathError{Op: "read", Path: path, Err: err}
	}
	if len(content) > maxConfigFileBytes {
		return nil, &os.PathError{Op: "read", Path: path, Err: errors.New("file too large")}
	}
	return content, nil
}

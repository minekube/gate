package reload

import (
	"os"
	"path/filepath"
	"syscall"
)

func openFingerprintFile(path string) (*os.File, error) {
	handle, err := createFingerprintHandle(path)
	if err != nil {
		// Match os.Open's long-path behavior on modern Windows first. Older
		// environments still need the extended form when the ordinary open
		// exceeds the legacy path limit.
		longPath := fixFingerprintPath(path)
		if longPath != path {
			handle, err = createFingerprintHandle(longPath)
		}
	}
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}

func createFingerprintHandle(path string) (syscall.Handle, error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	// An editor may replace the config while reconciliation hashes it. Windows
	// permits that only when every open handle opts into delete sharing.
	handle, err := syscall.CreateFile(
		pathp,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return handle, nil
}

func fixFingerprintPath(path string) string {
	if path == "" || hasExtendedPathPrefix(path) {
		return path
	}

	pathLength := len(path)
	if !filepath.IsAbs(path) {
		workingDir, err := os.Getwd()
		if err != nil {
			return path
		}
		pathLength += len(workingDir) + 1
	}
	if pathLength < 248 {
		return path
	}

	fullPath, err := syscall.FullPath(path)
	if err != nil {
		return path
	}
	if hasExtendedPathPrefix(fullPath) {
		return fullPath
	}
	if len(fullPath) >= 2 && isFingerprintPathSeparator(fullPath[0]) && isFingerprintPathSeparator(fullPath[1]) {
		if len(fullPath) >= 4 && fullPath[2] == '.' && isFingerprintPathSeparator(fullPath[3]) {
			return fullPath
		}
		return `\\?\UNC\` + fullPath[2:]
	}
	return `\\?\` + fullPath
}

func hasExtendedPathPrefix(path string) bool {
	if len(path) < 4 {
		return false
	}
	if path[:4] == `\??\` {
		return true
	}
	return isFingerprintPathSeparator(path[0]) && isFingerprintPathSeparator(path[1]) && path[2] == '?' && isFingerprintPathSeparator(path[3])
}

func isFingerprintPathSeparator(char byte) bool {
	return char == '\\' || char == '/'
}

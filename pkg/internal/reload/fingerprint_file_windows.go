package reload

import (
	"os"
	"syscall"
)

func openFingerprintFile(path string) (*os.File, error) {
	handle, err := createFingerprintHandle(path)
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

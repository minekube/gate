//go:build windows

package reload

import (
	"syscall"
	"unsafe"
)

var replaceFileW = syscall.NewLazyDLL("kernel32.dll").NewProc("ReplaceFileW")

func replaceFingerprintTestFile(source, destination string) error {
	destinationPath, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	sourcePath, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	result, _, callErr := replaceFileW.Call(
		uintptr(unsafe.Pointer(destinationPath)),
		uintptr(unsafe.Pointer(sourcePath)),
		0,
		0,
		0,
		0,
	)
	if result == 0 {
		return callErr
	}
	return nil
}

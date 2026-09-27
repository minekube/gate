//go:build windows

package gate

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/gate"
)

// replaceFileW is kernel32!ReplaceFileW - the API Windows provides for replacing
// a file another process has open, i.e. what an editor or deploy tool does when
// it saves config.yml. It is refused with ERROR_SHARING_VIOLATION unless every
// open handle on the destination shares delete, which is exactly what this file
// measures Gate's startup config read against.
// (pkg/internal/reload has the same helper for the watcher and reload reads.)
var replaceFileW = syscall.NewLazyDLL("kernel32.dll").NewProc("ReplaceFileW")

func replaceConfigFile(source, destination string) error {
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

// TestStartupConfigLoadAllowsReplacementsWhileLoading measures the startup path
// the residual card is about: a goroutine loads the config the way `gate` does
// when no --config was given (initViper discovers ./config.yml and names it on
// viper, LoadConfig reads it) while ReplaceFileW replacements of that file - what
// an editor does when it saves it - are attempted. Gate may not refuse any of
// them, i.e. every read it makes of the config must share delete.
//
// The control below is deliberate: an unshared handle (what viper's own read
// opens) refuses the very same replacement on this runner, so a zero-refusal
// result is a property of Gate's read and not of a runner where the replacement
// cannot be refused at all.
//
// Each iteration builds a fresh viper like a fresh `gate` process does: viper
// caches the discovered file after its first successful read, so a reused viper
// would only ever exercise viper's own read once.
func TestStartupConfigLoadAllowsReplacementsWhileLoading(t *testing.T) {
	const (
		minLoads    = 100
		maxAttempts = 10000
		timeout     = 60 * time.Second
	)
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "config.yml")
	temporary := filepath.Join(dir, "config.yml.tmp")
	content := "config:\n  bind: 127.0.0.1:25565\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(temporary, []byte(content), 0o600))

	// Control: the same replacement against a handle that does not share delete.
	unshared, err := os.Open(path)
	require.NoError(t, err)
	require.Error(t, replaceConfigFile(temporary, path),
		"an unshared handle must refuse this replacement on this runner - the control for the loader below")
	require.NoError(t, unshared.Close())

	resetGlobalViper(t)
	ctx := configlessContext(t)

	stop := make(chan struct{})
	loaderDone := make(chan struct{})
	var loads, withoutFile atomic.Int64
	go func() {
		defer close(loaderDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			// ReplaceFileW drops the destination name for the duration of the
			// swap, so a load can observe a transient absence (or a file being
			// replaced mid-read) and fail; only the refusals below are this
			// test's subject.
			gate.Viper = gate.NewViper()
			v, err := initViper(ctx, "")
			if err != nil {
				continue
			}
			if v.ConfigFileUsed() == "" {
				// Discovery ran inside a swap window and found nothing, so this
				// load falls back to viper's own discovery read.
				withoutFile.Add(1)
			}
			if _, err := gate.LoadConfig(v); err == nil {
				loads.Add(1)
			}
		}
	}()

	refused := 0
	attempts := 0
	var firstRefusal error
	var firstRefusalCode uintptr
	deadline := time.Now().Add(timeout)
	for attempts < maxAttempts && time.Now().Before(deadline) {
		require.NoError(t, os.WriteFile(temporary, []byte(content), 0o600))
		attempts++
		if err := replaceConfigFile(temporary, path); err != nil {
			refused++
			if firstRefusal == nil {
				firstRefusal = err
				if errno, ok := err.(syscall.Errno); ok {
					firstRefusalCode = uintptr(errno)
				}
			}
		}
	}
	close(stop)
	<-loaderDone

	t.Logf("loaded config %d times over %d ReplaceFileW attempts, refusals %d (first: %v, code %d), loads whose discovery found no file %d",
		loads.Load(), attempts, refused, firstRefusal, firstRefusalCode, withoutFile.Load())
	require.Zero(t, refused,
		"Gate's startup config read must not refuse an operator's atomic config replacement")
	require.GreaterOrEqual(t, loads.Load(), int64(minLoads),
		"the loads must have run alongside the replacement attempts, or the zero above says nothing")
}

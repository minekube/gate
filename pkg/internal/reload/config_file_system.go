package reload

import (
	"os"

	"github.com/spf13/afero"
)

// OpenConfigFile opens one config file for reading with the delete-sharing
// handle Gate reads config content with (openFingerprintFile, the share mode
// gate#959 introduced for the watcher). On Windows a handle that does not share
// delete makes an editor's atomic replacement of that file - ReplaceFileW, the
// API Windows provides for replacing a file another process has open - fail with
// ERROR_SHARING_VIOLATION for as long as the handle is open, i.e. it is Gate
// refusing an operator's config save; on other platforms this is os.Open. The
// caller owns the returned file and closes it.
func OpenConfigFile(path string) (*os.File, error) {
	return openFingerprintFile(path)
}

// ConfigFileSystem returns the file system Gate reads config files through: the
// OS file system with read-only opens replaced by OpenConfigFile. Gate reads
// config content with ReadConfigFile, but a library can still read the same file
// on its own - viper's ReadInConfig reads the file its own discovery picked,
// which is how `gate` loaded its config before Gate resolved the path itself -
// and such a read must share delete too, or it refuses an operator's atomic
// config replacement on Windows for as long as it is in flight.
//
// Only read-only opens change: discovery (Stat), writes (Create/OpenFile with
// write flags) and everything else stay the OS file system's, so which file is
// found, what it contains and what a write produces are unchanged.
func ConfigFileSystem() afero.Fs { return configFileSystem{Fs: afero.NewOsFs()} }

type configFileSystem struct{ afero.Fs }

func (f configFileSystem) Open(name string) (afero.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

func (f configFileSystem) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_TRUNC) == 0 {
		return OpenConfigFile(name)
	}
	return f.Fs.OpenFile(name, flag, perm)
}

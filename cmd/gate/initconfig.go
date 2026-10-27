package gate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.minekube.com/gate/pkg/configs"
)

// firstStartConfigFileName is the config file Gate generates when it was asked
// to and found none. It is the name its own discovery reads (initViper), so a
// generated file is the one the next start loads without any flag.
const firstStartConfigFileName = "config.yml"

// firstStartResult reports what the optional first-start write did.
type firstStartResult struct {
	// Written is the path of the config file Gate generated in this run, or ""
	// when it generated none.
	Written string
	// Skipped is true when a config file was already there. It is never
	// touched: generation only ever creates a missing file.
	Skipped bool
	// Warn is the error of a write that was attempted and failed. It is not
	// fatal - Gate boots on the built-in defaults either way.
	Warn error
}

// firstStartConfig performs the opt-in first-start config write
// (--init-config / GATE_INIT_CONFIG).
//
// The file it writes is the minimal template: a file that states only defaults.
// Generating one must not change how Gate behaves - it gives the operator
// something to edit and gives Gate a file to watch for reloads. The annotated
// templates stay opt-in through `gate config --type ...`.
//
// explicitConfig is the -c/--config value ("" when the operator passed none).
// With one, the generated file goes to that path, which also satisfies a
// missing explicit config without hand-writing a file. Without one, the target
// is config.yml in the working directory: the file Gate's own discovery reads.
//
// The returned error is only for a request that cannot be placed anywhere -
// there is no working directory to resolve ./config.yml in. A target that
// already exists or cannot be written is reported in the result instead, so the
// caller can log it and keep booting.
func firstStartConfig(explicitConfig string, generate bool) (firstStartResult, error) {
	if !generate {
		return firstStartResult{}, nil
	}

	target := explicitConfig
	if target == "" {
		if existing := findDefaultConfigFile(); existing != "" {
			return firstStartResult{Skipped: true}, nil
		}
		wd, err := os.Getwd()
		if err != nil {
			return firstStartResult{}, fmt.Errorf("cannot write %s: no working directory to place it in: %w",
				firstStartConfigFileName, err)
		}
		target = filepath.Join(wd, firstStartConfigFileName)
	}

	if err := writeConfigFile(target, configs.MinimalConfigBytes, false); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return firstStartResult{Skipped: true}, nil
		}
		return firstStartResult{Warn: err}, nil
	}
	return firstStartResult{Written: target}, nil
}

// writeConfigFile writes content to path. Without overwrite it refuses to
// replace an existing file (O_CREATE|O_EXCL), so a config file an operator
// wrote is never truncated by a command or a start that generates one.
// overwrite truncates instead and only exists behind an explicit flag.
func writeConfigFile(path string, content []byte, overwrite bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		// The file is ours - O_EXCL created it - so do not leave a truncated
		// config behind that the next start would load.
		if !overwrite {
			_ = os.Remove(path)
		}
		return err
	}
	return f.Close()
}

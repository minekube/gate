package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/wasm/generate"
)

func TestRunGenerateAndCheck(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	output := t.TempDir()
	nativeOutput := t.TempDir()

	var stdout bytes.Buffer
	require.NoError(t, run([]string{
		"generate",
		"-repo", root,
		"-out", output,
		"-native-out", nativeOutput,
	}, &stdout, &stdout))
	require.Contains(t, stdout.String(), "generated 6 WebAssembly API artifacts")
	for _, name := range []string{
		generate.WITFile,
		generate.ManifestFile,
		generate.ContractFile,
		generate.GoValuesFile,
		generate.GoDispatchFile,
	} {
		info, err := os.Stat(filepath.Join(output, name))
		require.NoError(t, err)
		require.Positive(t, info.Size())
	}
	info, err := os.Stat(filepath.Join(nativeOutput, generate.RustValuesFile))
	require.NoError(t, err)
	require.Positive(t, info.Size())

	stdout.Reset()
	require.NoError(t, run([]string{
		"check",
		"-repo", root,
		"-out", output,
		"-native-out", nativeOutput,
	}, &stdout, &stdout))
	require.Contains(t, stdout.String(), "WebAssembly API artifacts are current")

	witPath := filepath.Join(output, generate.WITFile)
	require.NoError(t, os.WriteFile(witPath, []byte("stale"), 0o644))
	stdout.Reset()
	err = run([]string{
		"check",
		"-repo", root,
		"-out", output,
		"-native-out", nativeOutput,
	}, &stdout, &stdout)
	require.ErrorContains(t, err, "gate.wit differs")
	contents, readErr := os.ReadFile(witPath)
	require.NoError(t, readErr)
	require.Equal(t, "stale", string(contents), "check must not modify artifacts")
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"}, &bytes.Buffer{}, &bytes.Buffer{})
	require.ErrorContains(t, err, "usage:")
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/internal/builtin/wasm/codegen/generate"
)

func TestRunGenerateAndCheck(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", ".."))
	require.NoError(t, err)
	output := t.TempDir()
	nativeOutput := t.TempDir()
	publicOutput := t.TempDir()

	var stdout bytes.Buffer
	require.NoError(t, run([]string{
		"generate",
		"-repo", root,
		"-out", output,
		"-native-out", nativeOutput,
		"-public-out", publicOutput,
	}, &stdout, &stdout))
	require.Contains(t, stdout.String(), "generated 12 WebAssembly API artifacts")
	for _, name := range []string{
		generate.WITFile,
		generate.ManifestFile,
		generate.ContractFile,
		generate.GoValuesFile,
		generate.GoDispatchFile,
		generate.CHeaderFile,
	} {
		info, err := os.Stat(filepath.Join(output, name))
		require.NoError(t, err)
		require.Positive(t, info.Size())
	}
	for _, name := range []string{
		generate.WITFile,
		generate.ContractFile,
	} {
		internal, err := os.ReadFile(filepath.Join(output, name))
		require.NoError(t, err)
		public, err := os.ReadFile(filepath.Join(publicOutput, name))
		require.NoError(t, err)
		require.Equal(t, internal, public)
	}
	_, err = os.Stat(filepath.Join(publicOutput, generate.GoValuesFile))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, name := range []string{
		generate.RustValuesFile,
		generate.RustBindingsFile,
		generate.RustDispatchFile,
	} {
		info, err := os.Stat(filepath.Join(nativeOutput, name))
		require.NoError(t, err)
		require.Positive(t, info.Size())
	}

	stdout.Reset()
	require.NoError(t, run([]string{
		"check",
		"-repo", root,
		"-out", output,
		"-native-out", nativeOutput,
		"-public-out", publicOutput,
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
		"-public-out", publicOutput,
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

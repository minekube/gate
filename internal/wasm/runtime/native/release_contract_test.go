package native_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseAndContainerBuildsLinkTheNativeRuntime(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..", "..", "..")
	read := func(name string) string {
		t.Helper()
		value, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return string(value)
	}

	dockerfile := read("Dockerfile")
	require.Contains(t, dockerfile, "FROM --platform=$TARGETPLATFORM")
	require.Contains(t, dockerfile, "cargo build -p gate-wasm-native --release")
	require.Contains(t, dockerfile, "CGO_ENABLED=1")
	require.Contains(t, dockerfile, "-tags=wasm_native")
	require.Contains(t, dockerfile, "libgcc_s.so.1")

	workflow := read(filepath.Join(".github", "workflows", "ci.yml"))
	for _, target := range []string{
		"goos: linux\n            goarch: amd64",
		"goos: linux\n            goarch: arm64",
		"goos: darwin\n            goarch: amd64",
		"goos: darwin\n            goarch: arm64",
		"goos: windows\n            goarch: amd64",
	} {
		require.Contains(t, workflow, target)
	}
	require.Contains(t, workflow, "go build -tags=wasm_native")
	require.Contains(t, workflow, "- wasm-native-feasibility")
	require.Contains(t, workflow, "msys2/setup-msys2@v2")
	require.Contains(t, workflow, "gate_wasm_contract_${version}.tar.gz")
	require.Contains(t, workflow, "make wasm-rust-example-test")

	releaser := read(".goreleaser.yml")
	require.Contains(t, releaser, "CGO_ENABLED=1")
	require.Contains(t, releaser, "wasm_native")
	require.NotContains(t, releaser, "CGO_ENABLED=0")
}

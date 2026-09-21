//go:build tools

package main

// Keep the dependencies used by the explicit-file pinned-tool guard tests in
// the root module graph. Go package discovery ignores .github/, while CI runs
// those files directly with `go test` and `go run`.
import (
	_ "golang.org/x/mod/semver"
	_ "mvdan.cc/sh/v3/syntax"
)

#!/bin/sh
set -eu

deps=$(go list -tags musl -deps ./cmd/gate)

if printf '%s\n' "$deps" | grep -Eq '^(github.com/ebitengine/purego|go.minekube.com/geyserlite|github.com/honeycombio/otel-config-go/otelconfig)$'; then
    echo "musl build still depends on geyserlite, purego, or Honeycomb OTEL auto-config" >&2
    exit 1
fi

go test -tags musl ./cmd/gate ./pkg/internal/otelutil ./pkg/edition/bedrock/geyser

// Package configs provides embedded default configuration files.
// Run `go generate ./pkg/configs` or `make sync-configs` to update the embedded configs from the root directory.
package configs

//go:generate cp ../../config.yml config.yml
//go:generate cp ../../config-simple.yml config-simple.yml
//go:generate cp ../../config-lite.yml config-lite.yml
//go:generate cp ../../config-bedrock.yml config-bedrock.yml
// Note: config-minimal.yml is maintained directly in this directory, not synced from root

import _ "embed"

// Embedded configuration files for the `gate config` command.
// These are the default configuration templates that ship with Gate.

//go:embed config.yml
var DefaultConfigBytes []byte

//go:embed config-simple.yml
var SimpleConfigBytes []byte

//go:embed config-lite.yml
var LiteConfigBytes []byte

//go:embed config-bedrock.yml
var BedrockConfigBytes []byte

//go:embed config-minimal.yml
var MinimalConfigBytes []byte

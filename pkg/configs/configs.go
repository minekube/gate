// Package configs provides embedded default configuration files.
// Run `go generate ./pkg/configs` or `make sync-configs` to update the embedded configs from the root directory.
package configs

//go:generate cp ../../config.yml config.yml

import _ "embed"

// Embedded configuration files for the `gate config` command.
// These are the default configuration templates that ship with Gate.

//go:embed config.yml
var DefaultConfigBytes []byte

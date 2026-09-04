//go:build musl

package geyser

import (
	"context"
	"fmt"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
)

type liteManagedRunner struct{}

func newLiteManagedRunner(*config.Config) *liteManagedRunner {
	return &liteManagedRunner{}
}

func (r *liteManagedRunner) EnsureKey(context.Context) error {
	return fmt.Errorf("managed geyserlite engine is not available in this Gate build; set bedrock.managed.engine to %q or use the standard glibc Linux build", config.ManagedEngineJava)
}

func (r *liteManagedRunner) Start(context.Context) error {
	return fmt.Errorf("managed geyserlite engine is not available in this Gate build; set bedrock.managed.engine to %q or use the standard glibc Linux build", config.ManagedEngineJava)
}

func (r *liteManagedRunner) Stop() {}

// Done and Err preserve the managedRunner lifecycle contract on musl builds.
// The runtime never starts on this platform, so it has no terminal lifecycle
// signal to report.
func (r *liteManagedRunner) Done() <-chan struct{} { return nil }

func (r *liteManagedRunner) Err() error { return nil }

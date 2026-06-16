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

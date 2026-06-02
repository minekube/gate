//go:build windows

package geyser

import (
	"context"
	"fmt"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
)

type liteManagedRunner struct {
	cfg *config.Config
}

func newLiteManagedRunner(cfg *config.Config) *liteManagedRunner {
	return &liteManagedRunner{cfg: cfg}
}

func (r *liteManagedRunner) EnsureKey(context.Context) error {
	return fmt.Errorf("managed geyserlite is not supported on windows; set bedrock.managed.engine to %q to use java geyser", config.ManagedEngineJava)
}

func (r *liteManagedRunner) Start(context.Context) error {
	return fmt.Errorf("managed geyserlite is not supported on windows; set bedrock.managed.engine to %q to use java geyser", config.ManagedEngineJava)
}

func (r *liteManagedRunner) Stop() {}

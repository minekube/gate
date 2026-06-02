//go:build windows

package geyser

import (
	"context"
	"strings"
	"testing"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
)

func TestNewManagedRunnerDefaultsToUnsupportedGeyserliteOnWindows(t *testing.T) {
	cfg := &config.Config{
		GeyserListenAddr: "localhost:25567",
		FloodgateKeyPath: "floodgate.pem",
		Managed: &config.ManagedGeyser{
			Enabled: true,
		},
	}

	runner, err := newManagedRunner(cfg)
	if err != nil {
		t.Fatalf("newManagedRunner() error = %v", err)
	}

	if _, ok := runner.(*liteManagedRunner); !ok {
		t.Fatalf("newManagedRunner() = %T, want *liteManagedRunner", runner)
	}

	err = runner.EnsureKey(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not supported on windows") {
		t.Fatalf("EnsureKey() error = %v, want unsupported windows error", err)
	}
}

func TestNewManagedRunnerKeepsJavaEngineOnWindows(t *testing.T) {
	cfg := &config.Config{
		GeyserListenAddr: "localhost:25567",
		FloodgateKeyPath: "floodgate.pem",
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  config.ManagedEngineJava,
		},
	}

	runner, err := newManagedRunner(cfg)
	if err != nil {
		t.Fatalf("newManagedRunner() error = %v", err)
	}

	if _, ok := runner.(*javaManagedRunner); !ok {
		t.Fatalf("newManagedRunner() = %T, want *javaManagedRunner", runner)
	}
}

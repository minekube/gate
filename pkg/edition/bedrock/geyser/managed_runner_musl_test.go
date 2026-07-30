//go:build musl

package geyser

import (
	"context"
	"strings"
	"testing"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
)

func TestNoGeyserliteBuildRejectsGeyserliteEngine(t *testing.T) {
	runner, err := newManagedRunner(&config.Config{
		FloodgateKeyPath: "floodgate.pem",
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  config.ManagedEngineGeyserlite,
		},
	})
	if err != nil {
		t.Fatalf("newManagedRunner() error = %v", err)
	}

	err = runner.EnsureKey(context.Background())
	if err == nil {
		t.Fatal("EnsureKey() error = nil, want unavailable geyserlite error")
	}
	if !strings.Contains(err.Error(), "managed geyserlite engine is not available") {
		t.Fatalf("EnsureKey() error = %q, want unavailable geyserlite error", err)
	}
}

func TestNoGeyserliteBuildKeepsJavaEngine(t *testing.T) {
	runner, err := newManagedRunner(&config.Config{
		FloodgateKeyPath: "floodgate.pem",
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  config.ManagedEngineJava,
		},
	})
	if err != nil {
		t.Fatalf("newManagedRunner() error = %v", err)
	}
	if _, ok := runner.(*javaManagedRunner); !ok {
		t.Fatalf("newManagedRunner() = %T, want *javaManagedRunner", runner)
	}
}

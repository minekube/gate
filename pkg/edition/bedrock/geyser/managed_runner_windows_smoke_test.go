//go:build windows && geyserlite_smoke

package geyser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
)

func TestLiteManagedRunnerWindowsAutoDownloadSmoke(t *testing.T) {
	t.Setenv("GEYSERLITE_BINARY", "")
	t.Setenv("GEYSERLITE_LIBRARY", "")

	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "floodgate.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	cfg := &config.Config{
		GeyserListenAddr: "127.0.0.1:25567",
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  config.ManagedEngineGeyserlite,
			Mode:    "subprocess",
			ConfigOverrides: map[string]any{
				"bedrock": map[string]any{
					"address": "127.0.0.1",
					"port":    19142,
				},
			},
		},
	}

	runner := newLiteManagedRunner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if err := runner.Start(ctx); err != nil {
		t.Fatalf("start managed geyserlite: %v", err)
	}
	t.Cleanup(runner.Stop)
}

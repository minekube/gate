//go:build !windows

package geyser

import (
	"os"
	"path/filepath"
	"testing"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	geyserlite "go.minekube.com/geyserlite"
)

func TestNewManagedRunnerDefaultsToGeyserlite(t *testing.T) {
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
}

func TestNewManagedRunnerKeepsJavaEngine(t *testing.T) {
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

func TestNewManagedRunnerRejectsUnknownEngine(t *testing.T) {
	cfg := &config.Config{
		GeyserListenAddr: "localhost:25567",
		FloodgateKeyPath: "floodgate.pem",
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  "bogus",
		},
	}

	if _, err := newManagedRunner(cfg); err == nil {
		t.Fatal("newManagedRunner() error = nil, want unknown engine error")
	}
}

func TestLiteManagedRunnerOptionsMapManagedConfig(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "floodgate.key")
	key := []byte("0123456789abcdef")
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	cfg := &config.Config{
		GeyserListenAddr: "localhost:25567",
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled:     true,
			Engine:      config.ManagedEngineGeyserlite,
			Mode:        "embedded",
			LibraryPath: "/opt/geyserlite/libgeyserlite.so",
			BinaryPath:  "/opt/geyserlite/geyserlite",
			Mirror:      "https://mirror.example.com/geyserlite",
			Version:     "v0.2.0",
			Offline:     true,
			ExtraArgs:   []string{"-Xmx512m"},
			ConfigOverrides: map[string]any{
				"bedrock": map[string]any{"port": 19133},
			},
		},
	}

	runner := newLiteManagedRunner(cfg)
	opts, err := runner.options()
	if err != nil {
		t.Fatalf("options() error = %v", err)
	}

	if opts.Upstream != "localhost:25567" {
		t.Fatalf("Upstream = %q, want localhost:25567", opts.Upstream)
	}
	if opts.AuthType != geyserlite.Floodgate {
		t.Fatalf("AuthType = %v, want Floodgate", opts.AuthType)
	}
	if string(opts.FloodgateKey) != string(key) {
		t.Fatalf("FloodgateKey = %q, want %q", opts.FloodgateKey, key)
	}
	if opts.Mode != geyserlite.ModeEmbedded {
		t.Fatalf("Mode = %v, want ModeEmbedded", opts.Mode)
	}
	if opts.LibraryPath != "/opt/geyserlite/libgeyserlite.so" {
		t.Fatalf("LibraryPath = %q", opts.LibraryPath)
	}
	if opts.BinaryPath != "/opt/geyserlite/geyserlite" {
		t.Fatalf("BinaryPath = %q", opts.BinaryPath)
	}
	if opts.Mirror != "https://mirror.example.com/geyserlite" {
		t.Fatalf("Mirror = %q", opts.Mirror)
	}
	if opts.Version != "v0.2.0" {
		t.Fatalf("Version = %q", opts.Version)
	}
	if !opts.Offline {
		t.Fatal("Offline = false, want true")
	}
	if len(opts.JVMArgs) != 1 || opts.JVMArgs[0] != "-Xmx512m" {
		t.Fatalf("JVMArgs = %#v", opts.JVMArgs)
	}
	if opts.ConfigOverrides["bedrock"] == nil {
		t.Fatalf("ConfigOverrides = %#v, want bedrock override", opts.ConfigOverrides)
	}
}

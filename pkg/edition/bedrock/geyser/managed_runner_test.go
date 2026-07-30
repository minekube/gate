//go:build !musl

package geyser

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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
			Version:     "v0.2.1",
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
	if opts.Version != "v0.2.1" {
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

func TestLiteManagedRunnerDefaultsUseSubprocessAutoDownload(t *testing.T) {
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
			Enabled: true,
			Engine:  config.ManagedEngineGeyserlite,
		},
	}

	runner := newLiteManagedRunner(cfg)
	opts, err := runner.options()
	if err != nil {
		t.Fatalf("options() error = %v", err)
	}

	if opts.Mode != geyserlite.ModeSubprocess {
		t.Fatalf("Mode = %v, want ModeSubprocess", opts.Mode)
	}
	if opts.LibraryPath != "" {
		t.Fatalf("LibraryPath = %q, want empty for auto-download subprocess mode", opts.LibraryPath)
	}
	if opts.BinaryPath != "" {
		t.Fatalf("BinaryPath = %q, want empty for auto-download subprocess mode", opts.BinaryPath)
	}
	if opts.Version != "" {
		t.Fatalf("Version = %q, want empty to use geyserlite.DefaultVersion", opts.Version)
	}
	if opts.Mirror != "" {
		t.Fatalf("Mirror = %q, want empty to use geyserlite.DefaultDownloadBase", opts.Mirror)
	}
	if opts.Offline {
		t.Fatal("Offline = true, want false so auto-download can fetch release assets")
	}
}

func TestLiteManagedRunnerStartWaitsUntilHealthy(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "floodgate.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	cfg := &config.Config{
		GeyserListenAddr: "localhost:25567",
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  config.ManagedEngineGeyserlite,
		},
	}

	fake := &fakeGeyserliteServer{started: make(chan struct{})}
	runner := newLiteManagedRunner(cfg)
	runner.newServer = func(geyserlite.Options) (geyserliteServer, error) {
		return fake, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Start(ctx) }()

	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatal("geyserlite server did not start")
	}

	select {
	case err := <-errCh:
		t.Fatalf("Start returned before Healthy: %v", err)
	case <-time.After(2 * liteManagedReadyPoll):
	}

	fake.healthy.Store(true)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error after Healthy: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after Healthy")
	}

	runner.Stop()
}

type fakeGeyserliteServer struct {
	healthy atomic.Bool
	started chan struct{}
}

func (f *fakeGeyserliteServer) Start(ctx context.Context) error {
	close(f.started)
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeGeyserliteServer) Stop(context.Context) error { return nil }

func (f *fakeGeyserliteServer) Healthy() bool { return f.healthy.Load() }

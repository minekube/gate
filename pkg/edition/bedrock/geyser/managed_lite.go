package geyser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	geyserlite "go.minekube.com/geyserlite"
)

type liteManagedRunner struct {
	cfg    *config.Config
	server *geyserlite.Server
	cancel context.CancelFunc
	done   chan error
	mu     sync.Mutex
}

func newLiteManagedRunner(cfg *config.Config) *liteManagedRunner {
	return &liteManagedRunner{cfg: cfg}
}

func (r *liteManagedRunner) EnsureKey(ctx context.Context) error {
	log := slog.Default().With("component", "geyserlite.managed")
	keyPath := r.cfg.FloodgateKeyPath
	if keyPath == "" {
		return fmt.Errorf("floodgate key path not configured")
	}
	if info, err := os.Stat(keyPath); err == nil && !info.IsDir() {
		log.DebugContext(ctx, "floodgate key already exists", "path", keyPath)
		return nil
	}
	log.InfoContext(ctx, "generating floodgate key", "path", keyPath)
	if err := floodgate.GenerateKeyToFile(keyPath); err != nil {
		return fmt.Errorf("failed to generate floodgate key: %w", err)
	}
	return nil
}

func (r *liteManagedRunner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		return fmt.Errorf("geyserlite already running")
	}

	opts, err := r.options()
	if err != nil {
		return err
	}
	server, err := geyserlite.New(opts)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	r.server = server
	r.cancel = cancel
	r.done = done

	go func() {
		err := server.Start(runCtx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			err = nil
		}
		done <- err
	}()

	select {
	case err := <-done:
		r.server = nil
		r.cancel = nil
		r.done = nil
		return err
	case <-time.After(250 * time.Millisecond):
		return nil
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

func (r *liteManagedRunner) Stop() {
	r.mu.Lock()
	server := r.server
	cancel := r.cancel
	done := r.done
	r.server = nil
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()

	if server == nil {
		return
	}
	if cancel != nil {
		cancel()
	}

	ctx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStop()
	_ = server.Stop(ctx)
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}

func (r *liteManagedRunner) options() (geyserlite.Options, error) {
	managed := r.cfg.GetManaged()
	key, err := os.ReadFile(r.cfg.FloodgateKeyPath)
	if err != nil {
		return geyserlite.Options{}, fmt.Errorf("failed to read floodgate key: %w", err)
	}

	return geyserlite.Options{
		Upstream:        r.cfg.GeyserListenAddr,
		AuthType:        geyserlite.Floodgate,
		FloodgateKey:    key,
		Mode:            geyserliteMode(managed.Mode),
		LibraryPath:     managed.LibraryPath,
		BinaryPath:      managed.BinaryPath,
		JVMArgs:         managed.ExtraArgs,
		Logger:          slog.Default(),
		Version:         managed.Version,
		Mirror:          managed.Mirror,
		Offline:         managed.Offline,
		ConfigOverrides: managed.ConfigOverrides,
	}, nil
}

func geyserliteMode(mode string) geyserlite.Mode {
	switch mode {
	case "embedded":
		return geyserlite.ModeEmbedded
	default:
		return geyserlite.ModeSubprocess
	}
}

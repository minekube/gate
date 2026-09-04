//go:build !musl

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
	cfg       *config.Config
	newServer func(geyserlite.Options) (geyserliteServer, error)
	server    geyserliteServer
	cancel    context.CancelFunc
	done      chan struct{}
	err       error
	mu        sync.Mutex
}

func newLiteManagedRunner(cfg *config.Config) *liteManagedRunner {
	return &liteManagedRunner{
		cfg: cfg,
		newServer: func(opts geyserlite.Options) (geyserliteServer, error) {
			return geyserlite.New(opts)
		},
	}
}

type geyserliteServer interface {
	Start(context.Context) error
	Stop(context.Context) error
	Healthy() bool
}

const (
	liteManagedStartupTimeout = 10 * time.Minute
	liteManagedReadyPoll      = 100 * time.Millisecond
)

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
	if r.server != nil {
		r.mu.Unlock()
		return fmt.Errorf("geyserlite already running")
	}

	opts, err := r.options()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	server, err := r.newServer(opts)
	if err != nil {
		r.mu.Unlock()
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	r.server = server
	r.cancel = cancel
	r.done = done
	r.err = nil
	r.mu.Unlock()

	go func() {
		err := server.Start(runCtx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			err = nil
		}
		r.mu.Lock()
		if r.done == done {
			r.err = err
		}
		r.mu.Unlock()
		close(done)
	}()

	ticker := time.NewTicker(liteManagedReadyPoll)
	defer ticker.Stop()
	timeout := time.NewTimer(liteManagedStartupTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-done:
			err := r.runErr(done)
			r.clearRun(done)
			if err != nil {
				return err
			}
			return fmt.Errorf("geyserlite exited before becoming healthy")
		case <-ticker.C:
			if server.Healthy() {
				return nil
			}
		case <-timeout.C:
			cancel()
			r.clearRun(done)
			return fmt.Errorf("timed out after %s waiting for geyserlite to become healthy", liteManagedStartupTimeout)
		case <-ctx.Done():
			cancel()
			r.clearRun(done)
			return ctx.Err()
		}
	}
}

func (r *liteManagedRunner) clearRun(done <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done != done {
		return
	}
	r.server = nil
	r.cancel = nil
	r.done = nil
}

// Done closes once the managed GeyserLite runtime has stopped. It remains safe
// for more than one observer to wait on it, unlike the old one-consumer error
// channel.
func (r *liteManagedRunner) Done() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

func (r *liteManagedRunner) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *liteManagedRunner) runErr(done <-chan struct{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done != done {
		return nil
	}
	return r.err
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

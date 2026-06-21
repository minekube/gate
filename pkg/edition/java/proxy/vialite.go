//go:build !musl && !windows

package proxy

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/edition/java/config"
	vialite "go.minekube.com/vialite"
)

type vialiteServer interface {
	Start(context.Context) error
	WaitReady(context.Context) error
	Stop(context.Context) error
	Healthy() bool
	BackendDialAddress(name string) (string, error)
}

type viaManagedRunner struct {
	cfg            *config.Config
	newServer      func(vialite.Options) (vialiteServer, error)
	server         vialiteServer
	cancel         context.CancelFunc
	done           chan error
	activeBackends map[string]struct{}
	mu             sync.Mutex
}

func newViaManagedRunner(cfg *config.Config) *viaManagedRunner {
	return &viaManagedRunner{
		cfg: cfg,
		newServer: func(opts vialite.Options) (vialiteServer, error) {
			return vialite.New(opts)
		},
	}
}

func (r *viaManagedRunner) enabled() bool {
	return r != nil && r.cfg != nil && r.cfg.Via.Enabled && !r.cfg.Lite.Enabled
}

func (r *viaManagedRunner) backendEnabled(name string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.activeBackends[strings.ToLower(name)]
	return ok
}

func (r *viaManagedRunner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		return fmt.Errorf("vialite already running")
	}
	if !r.enabled() {
		return nil
	}
	opts, err := r.options()
	if err != nil {
		return err
	}
	server, err := r.newServer(opts)
	if err != nil {
		return err
	}
	activeBackends := make(map[string]struct{}, len(opts.Backends))
	for _, backend := range opts.Backends {
		activeBackends[strings.ToLower(backend.Name)] = struct{}{}
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	r.server = server
	r.cancel = cancel
	r.done = done
	go func() { done <- server.Start(runCtx) }()

	readyCtx, cancelReady := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelReady()
	select {
	case err := <-done:
		r.server = nil
		r.cancel = nil
		r.done = nil
		r.activeBackends = nil
		if err != nil {
			return err
		}
		return fmt.Errorf("vialite exited before becoming ready")
	default:
	}
	if err := server.WaitReady(readyCtx); err != nil {
		cancel()
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
		_ = server.Stop(stopCtx)
		cancelStop()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
		}
		r.server = nil
		r.cancel = nil
		r.done = nil
		r.activeBackends = nil
		return err
	}
	r.activeBackends = activeBackends
	return nil
}

func (r *viaManagedRunner) Stop() {
	r.mu.Lock()
	server := r.server
	cancel := r.cancel
	done := r.done
	r.server = nil
	r.cancel = nil
	r.done = nil
	r.activeBackends = nil
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

func (r *viaManagedRunner) BackendDialAddress(name string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("vialite is not running")
	}
	r.mu.Lock()
	server := r.server
	r.mu.Unlock()
	if server == nil {
		return "", fmt.Errorf("vialite is not running")
	}
	return server.BackendDialAddress(name)
}

func (r *viaManagedRunner) options() (vialite.Options, error) {
	opts := vialite.Options{
		Mode:        viaMode(r.cfg.Via.Mode),
		Bind:        r.cfg.Via.Bind,
		LibraryPath: r.cfg.Via.LibraryPath,
		BinaryPath:  r.cfg.Via.BinaryPath,
		Version:     r.cfg.Via.Version,
		Mirror:      r.cfg.Via.Mirror,
		Offline:     r.cfg.Via.Offline,
		Backends:    make([]vialite.Backend, 0, len(r.cfg.Servers)),
	}
	for name, addr := range r.cfg.Servers {
		opts.Backends = append(opts.Backends, vialite.Backend{
			Name:       name,
			Address:    addr,
			Forwarding: viaForwarding(r.cfg.Forwarding.Mode),
		})
	}
	return opts, nil
}

func viaMode(mode string) vialite.Mode {
	switch mode {
	case "subprocess":
		return vialite.ModeSubprocess
	default:
		return vialite.ModeEmbedded
	}
}

func viaForwarding(global config.ForwardingMode) vialite.ForwardingMode {
	switch global {
	case config.LegacyForwardingMode, config.BungeeGuardForwardingMode:
		return vialite.ForwardingLegacy
	case config.VelocityForwardingMode:
		return vialite.ForwardingVelocity
	default:
		return vialite.ForwardingNone
	}
}

type viaServerInfo struct {
	ServerInfo
	via *viaManagedRunner
}

func newViaServerInfo(info ServerInfo, via *viaManagedRunner) ServerInfo {
	return &viaServerInfo{ServerInfo: info, via: via}
}

func (i *viaServerInfo) Dial(ctx context.Context, player Player) (net.Conn, error) {
	addr, err := i.via.BackendDialAddress(i.Name())
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

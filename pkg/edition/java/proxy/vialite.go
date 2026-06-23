//go:build !musl

package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
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
	AddBackend(context.Context, vialite.Backend) (string, error)
	RemoveBackend(context.Context, string) error
}

type viaManagedRunner struct {
	cfg             *config.Config
	newServer       func(vialite.Options) (vialiteServer, error)
	server          vialiteServer
	cancel          context.CancelFunc
	done            chan error
	activeBackends  map[string]struct{}
	dynamicBackends map[string]*viaDynamicBackend
	mu              sync.Mutex
}

type viaDynamicBackend struct {
	bridge *viaBackendBridge
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
		r.dynamicBackends = nil
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
		r.dynamicBackends = nil
		return err
	}
	r.activeBackends = activeBackends
	r.dynamicBackends = make(map[string]*viaDynamicBackend)
	return nil
}

func (r *viaManagedRunner) Stop() {
	r.mu.Lock()
	server := r.server
	cancel := r.cancel
	done := r.done
	dynamicBackends := r.dynamicBackends
	r.server = nil
	r.cancel = nil
	r.done = nil
	r.activeBackends = nil
	r.dynamicBackends = nil
	r.mu.Unlock()

	for _, dynamic := range dynamicBackends {
		if dynamic != nil && dynamic.bridge != nil {
			_ = dynamic.bridge.Close()
		}
	}
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

func (r *viaManagedRunner) AddBackend(ctx context.Context, info ServerInfo) (bool, error) {
	if r == nil || info == nil {
		return false, nil
	}
	r.mu.Lock()
	server := r.server
	if server == nil {
		r.mu.Unlock()
		return false, nil
	}
	name := strings.ToLower(info.Name())
	if _, ok := r.activeBackends[name]; ok {
		r.mu.Unlock()
		return true, nil
	}
	r.mu.Unlock()

	backend, cleanup, err := r.dynamicBackend(info)
	if err != nil {
		return false, err
	}
	added := false
	defer func() {
		if !added && cleanup != nil {
			_ = cleanup.Close()
		}
	}()

	if _, err := server.AddBackend(ctx, backend); err != nil {
		if errors.Is(err, vialite.ErrDynamicBackendsUnsupported) {
			return false, nil
		}
		return false, err
	}

	r.mu.Lock()
	if r.server != server {
		r.mu.Unlock()
		_ = server.RemoveBackend(context.Background(), info.Name())
		return false, vialite.ErrNotStarted
	}
	if r.activeBackends == nil {
		r.activeBackends = make(map[string]struct{})
	}
	if r.dynamicBackends == nil {
		r.dynamicBackends = make(map[string]*viaDynamicBackend)
	}
	r.activeBackends[name] = struct{}{}
	r.dynamicBackends[name] = &viaDynamicBackend{}
	if cleanupBridge, ok := cleanup.(*viaBackendBridge); ok {
		r.dynamicBackends[name].bridge = cleanupBridge
	}
	added = true
	r.mu.Unlock()
	return true, nil
}

func (r *viaManagedRunner) dynamicBackend(info ServerInfo) (vialite.Backend, interface{ Close() error }, error) {
	address := info.Addr().String()
	var cleanup interface{ Close() error }
	if dialer, ok := info.(ServerDialer); ok {
		bridge, err := newViaBackendBridge(dialer)
		if err != nil {
			return vialite.Backend{}, nil, err
		}
		address = bridge.Addr().String()
		cleanup = bridge
	}
	return vialite.Backend{
		Name:       info.Name(),
		Address:    address,
		Forwarding: viaForwarding(r.cfg.Forwarding.Mode),
	}, cleanup, nil
}

func (r *viaManagedRunner) RemoveBackend(ctx context.Context, name string) error {
	if r == nil {
		return nil
	}
	key := strings.ToLower(name)
	r.mu.Lock()
	server := r.server
	if server == nil {
		r.mu.Unlock()
		return nil
	}
	if _, ok := r.dynamicBackends[key]; !ok {
		r.mu.Unlock()
		return nil
	}
	dynamic := r.dynamicBackends[key]
	r.mu.Unlock()

	err := server.RemoveBackend(ctx, name)
	if err != nil && !errors.Is(err, vialite.ErrBackendNotFound) {
		return err
	}

	r.mu.Lock()
	delete(r.dynamicBackends, key)
	delete(r.activeBackends, key)
	r.mu.Unlock()
	if dynamic != nil && dynamic.bridge != nil {
		_ = dynamic.bridge.Close()
	}
	return nil
}

func (r *viaManagedRunner) prepareBackendDial(ctx context.Context, name string, player Player) (func(), error) {
	if r == nil {
		return func() {}, nil
	}
	r.mu.Lock()
	dynamic := r.dynamicBackends[strings.ToLower(name)]
	r.mu.Unlock()
	if dynamic == nil || dynamic.bridge == nil {
		return func() {}, nil
	}
	return dynamic.bridge.Prepare(ctx, player)
}

func (r *viaManagedRunner) options() (vialite.Options, error) {
	opts := vialite.Options{
		Mode:                 viaMode(r.cfg.Via.Mode, runtime.GOOS, r.cfg.Via.LibraryPath),
		Bind:                 r.cfg.Via.Bind,
		LibraryPath:          r.cfg.Via.LibraryPath,
		BinaryPath:           r.cfg.Via.BinaryPath,
		Version:              r.cfg.Via.Version,
		Mirror:               r.cfg.Via.Mirror,
		Offline:              r.cfg.Via.Offline,
		AllowDynamicBackends: true,
		Backends:             make([]vialite.Backend, 0, len(r.cfg.Servers)),
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

func viaMode(mode, goos, libraryPath string) vialite.Mode {
	if mode == "" {
		return vialite.ModeSubprocess
	}
	if goos == "windows" && libraryPath == "" && mode == "embedded" {
		return vialite.ModeSubprocess
	}
	switch mode {
	case "embedded":
		return vialite.ModeEmbedded
	default:
		return vialite.ModeSubprocess
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
	cancelBridge, err := i.via.prepareBackendDial(ctx, i.Name(), player)
	if err != nil {
		return nil, err
	}
	addr, err := i.via.BackendDialAddress(i.Name())
	if err != nil {
		cancelBridge()
		return nil, err
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		cancelBridge()
		return nil, err
	}
	return conn, nil
}

type viaBridgeRequest struct {
	ctx    context.Context
	player Player
}

type viaBackendBridge struct {
	ln       net.Listener
	dialer   ServerDialer
	requests chan viaBridgeRequest
	done     chan struct{}
	close    sync.Once
}

func newViaBackendBridge(dialer ServerDialer) (*viaBackendBridge, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b := &viaBackendBridge{
		ln:       ln,
		dialer:   dialer,
		requests: make(chan viaBridgeRequest, 1024),
		done:     make(chan struct{}),
	}
	go b.accept()
	return b, nil
}

func (b *viaBackendBridge) Addr() net.Addr {
	return b.ln.Addr()
}

func (b *viaBackendBridge) Prepare(ctx context.Context, player Player) (func(), error) {
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	req := viaBridgeRequest{ctx: streamCtx, player: player}
	select {
	case b.requests <- req:
		return cancel, nil
	case <-b.done:
		cancel()
		return nil, net.ErrClosed
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

func (b *viaBackendBridge) Close() error {
	var err error
	b.close.Do(func() {
		close(b.done)
		err = b.ln.Close()
	})
	return err
}

func (b *viaBackendBridge) accept() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		go b.handle(conn)
	}
}

func (b *viaBackendBridge) handle(conn net.Conn) {
	defer conn.Close()
	var req viaBridgeRequest
	select {
	case req = <-b.requests:
	case <-b.done:
		return
	}
	if err := req.ctx.Err(); err != nil {
		return
	}
	backend, err := b.dialer.Dial(req.ctx, req.player)
	if err != nil {
		return
	}
	defer backend.Close()

	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(backend, conn)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(conn, backend)
		errCh <- err
	}()
	select {
	case <-errCh:
	case <-req.ctx.Done():
	case <-b.done:
	}
}

//go:build !musl

package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.minekube.com/gate/pkg/edition/java/config"
	javaversion "go.minekube.com/gate/pkg/edition/java/proto/version"
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
	done            chan struct{}
	exit            *viaRuntimeExit
	resolution      *viaRuntimeResolution
	death           *viaRuntimeDeath
	activeBackends  map[string]struct{}
	dynamicBackends map[string]*viaDynamicBackend
	mu              sync.Mutex
}

type viaDynamicBackend struct {
	bridge *viaBackendBridge
}

func newViaManagedRunner(cfg *config.Config) *viaManagedRunner {
	return &viaManagedRunner{
		cfg:        cfg,
		resolution: &viaRuntimeResolution{},
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
	done := make(chan struct{})
	exit := &viaRuntimeExit{}
	r.server = server
	r.cancel = cancel
	r.done = done
	r.exit = exit
	r.death = nil
	go func() {
		exit.set(server.Start(runCtx))
		close(done)
	}()

	readyCtx, cancelReady := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelReady()
	select {
	case <-done:
		err := exit.get()
		r.clearRuntimeLocked()
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
		r.clearRuntimeLocked()
		return err
	}
	r.activeBackends = activeBackends
	r.dynamicBackends = make(map[string]*viaDynamicBackend)
	// Startup succeeded. A runtime that dies afterwards used to be invisible:
	// Gate kept serving, and every join failed with the runtime's "server not
	// started" error while the backend was never contacted.
	go r.watchRuntimeDeath(server, runCtx, done, exit)
	return nil
}

// clearRuntimeLocked forgets the current runtime. Callers must hold r.mu.
func (r *viaManagedRunner) clearRuntimeLocked() {
	r.server = nil
	r.cancel = nil
	r.done = nil
	r.exit = nil
	r.death = nil
	r.activeBackends = nil
	r.dynamicBackends = nil
}

// watchRuntimeDeath reports a runtime that exited while Gate kept serving.
//
// It returns without reporting when Gate itself stopped the runtime (shutdown)
// or replaced it, and it reports at most once per runtime: the exit is recorded
// so that later joins fail with an actionable error instead of the runtime
// module's "server not started", which reads like a configuration mistake.
func (r *viaManagedRunner) watchRuntimeDeath(
	server vialiteServer,
	runCtx context.Context,
	done <-chan struct{},
	exit *viaRuntimeExit,
) {
	select {
	case <-done:
	case <-runCtx.Done():
		return // Gate is stopping the runtime; the shutdown path owns this.
	}
	if runCtx.Err() != nil {
		return // A deliberate stop raced with the runtime exiting.
	}
	cause := exit.get()
	if errors.Is(cause, context.Canceled) {
		cause = nil
	}
	r.mu.Lock()
	if r.server != server {
		r.mu.Unlock()
		return // A new runtime took over in the meantime.
	}
	backends := make([]string, 0, len(r.activeBackends))
	for name := range r.activeBackends {
		backends = append(backends, name)
	}
	sort.Strings(backends)
	death := &viaRuntimeDeath{
		version:  r.runtimeVersionLocked(),
		backends: backends,
		cause:    cause,
		at:       time.Now(),
	}
	r.death = death
	version := death.version
	r.mu.Unlock()

	attrs := []any{
		"version", version,
		"backends", strings.Join(backends, ","),
		"hint", "restart Gate to start a runtime again",
	}
	if cause != nil {
		attrs = append(attrs, "cause", cause.Error())
	}
	viaLogger().Error(
		"vialite: the managed runtime exited while Gate was serving; translated joins to its backends will fail",
		attrs...,
	)
}

// runtimeVersionLocked names the runtime artifact Gate is (or was) running: the
// version the runtime module reported on its "vialite: resolved runtime" line,
// falling back to the configured pin when the runtime never reported one (for
// example when an explicit binary path is used).
//
// Callers must hold r.mu.
func (r *viaManagedRunner) runtimeVersionLocked() string {
	if v := r.resolution.resolvedVersion(); v != "" {
		return v
	}
	if r.cfg != nil && r.cfg.Via.Version != "" {
		return r.cfg.Via.Version
	}
	return "unreported"
}

// runtimeDeath returns the recorded death of the current runtime, if any.
func (r *viaManagedRunner) runtimeDeath() *viaRuntimeDeath {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.death
}

func (r *viaManagedRunner) Stop() {
	r.mu.Lock()
	server := r.server
	cancel := r.cancel
	done := r.done
	dynamicBackends := r.dynamicBackends
	r.clearRuntimeLocked()
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
	death := r.death
	r.mu.Unlock()
	if server == nil {
		return "", fmt.Errorf("vialite is not running")
	}
	if death != nil {
		// The runtime is gone: report that, not the runtime module's "server not
		// started", which reads like a configuration mistake and sends operators
		// looking at the wrong thing.
		return "", death
	}
	return server.BackendDialAddress(name)
}

func (r *viaManagedRunner) AddBackend(ctx context.Context, info ServerInfo) (bool, error) {
	if r == nil || info == nil {
		return false, nil
	}
	r.mu.Lock()
	server := r.server
	death := r.death
	if server == nil {
		r.mu.Unlock()
		return false, nil
	}
	if death != nil {
		// The runtime died: adding a backend cannot work, and the runtime
		// module's "server not started" would name the wrong cause.
		r.mu.Unlock()
		return false, death
	}
	name := strings.ToLower(info.Name())
	if _, ok := r.activeBackends[name]; ok {
		r.mu.Unlock()
		return true, nil
	}
	r.mu.Unlock()
	if r.shouldSkipDynamicBackend(info) {
		return false, nil
	}

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

func (r *viaManagedRunner) shouldSkipDynamicBackend(info ServerInfo) bool {
	if info == nil {
		return false
	}
	versionProvider, hasBackendVersion := info.(BackendVersionProvider)
	clientProvider, hasClientProtocol := info.(ClientProtocolProvider)
	if !hasBackendVersion || !hasClientProtocol {
		return false
	}
	backendVersion := versionProvider.BackendVersion()
	if backendVersion == "" {
		return false
	}
	clientVersion := javaversion.Protocol(clientProvider.ClientProtocol()).Version()
	if clientVersion == nil || clientVersion == javaversion.Unknown || clientVersion == javaversion.Legacy {
		return false
	}
	for _, name := range clientVersion.Names {
		if name == backendVersion {
			return true
		}
	}
	return false
}

func (r *viaManagedRunner) dynamicBackend(info ServerInfo) (vialite.Backend, interface{ Close() error }, error) {
	address := info.Addr().String()
	version := ""
	forwarding := viaForwarding(r.cfg.Forwarding.Mode)
	if provider, ok := info.(BackendVersionProvider); ok {
		version = provider.BackendVersion()
	}
	if provider, ok := info.(ForwardingModeProvider); ok {
		forwarding = viaForwarding(provider.ForwardingMode())
	}
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
		Version:    version,
		Forwarding: forwarding,
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
		Logger:               r.logger(),
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

// viaLogger is the logger handed to the ViaLite runtime. The runtime logs one
// "resolved runtime" line per start naming the artifact version and where it
// came from (download, cache, embedded, local path), which is how an operator
// or supporter tells which runtime - and therefore which ViaVersion protocol
// ceiling - the proxy is actually running.
func viaLogger() *slog.Logger {
	return slog.Default().With("component", "vialite")
}

// logger is the runtime logger for this runner. It is the Gate logger above plus
// a small handler that remembers the version the runtime reports, so a
// runtime-death diagnostic can name the artifact that died instead of only the
// configured pin (which is empty for the default "latest" setup).
func (r *viaManagedRunner) logger() *slog.Logger {
	if r == nil || r.resolution == nil {
		return viaLogger()
	}
	return slog.New(viaResolutionHandler{Handler: slog.Default().Handler(), resolution: r.resolution}).
		With("component", "vialite")
}

// viaResolvedRuntimeMessage is the message the runtime module logs once per start
// naming the runtime artifact it resolved.
const viaResolvedRuntimeMessage = "vialite: resolved runtime"

// viaRuntimeResolution remembers the runtime version reported in the runtime
// module's "vialite: resolved runtime" line.
type viaRuntimeResolution struct {
	version atomic.Value // string
}

func (r *viaRuntimeResolution) recordVersion(version string) {
	if r != nil && version != "" {
		r.version.Store(version)
	}
}

func (r *viaRuntimeResolution) resolvedVersion() string {
	if r == nil {
		return ""
	}
	version, _ := r.version.Load().(string)
	return version
}

// viaResolutionHandler passes every log record through unchanged and, for the
// "vialite: resolved runtime" record, remembers its version attribute.
type viaResolutionHandler struct {
	slog.Handler
	resolution *viaRuntimeResolution
}

func (h viaResolutionHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Message == viaResolvedRuntimeMessage {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "version" {
				h.resolution.recordVersion(attr.Value.String())
				return false
			}
			return true
		})
	}
	return h.Handler.Handle(ctx, record)
}

func (h viaResolutionHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return viaResolutionHandler{Handler: h.Handler.WithAttrs(attrs), resolution: h.resolution}
}

func (h viaResolutionHandler) WithGroup(name string) slog.Handler {
	return viaResolutionHandler{Handler: h.Handler.WithGroup(name), resolution: h.resolution}
}

// viaRuntimeExit records how the runtime's Start call ended.
//
// Everything that waits for a runtime to stop - the startup readiness check,
// Gate's shutdown and the death watcher - observes the same result through it,
// instead of racing on a single channel receive.
type viaRuntimeExit struct {
	mu  sync.Mutex
	err error
}

func (e *viaRuntimeExit) set(err error) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.err = err
	e.mu.Unlock()
}

func (e *viaRuntimeExit) get() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.err
}

// viaRuntimeDeath describes a managed runtime that exited after Gate had started
// serving with it. It implements error so it can travel the normal join-failure
// path: the player's connection is closed with a reason that names the runtime
// death rather than the runtime module's "server not started".
type viaRuntimeDeath struct {
	version  string
	backends []string
	cause    error
	at       time.Time
}

func (d *viaRuntimeDeath) Error() string {
	if d.cause != nil {
		return fmt.Sprintf("vialite: the managed runtime (version %s) exited after startup (%v), "+
			"so protocol translation is down; restart Gate to recover", d.version, d.cause)
	}
	return fmt.Sprintf("vialite: the managed runtime (version %s) exited after startup, "+
		"so protocol translation is down; restart Gate to recover", d.version)
}

func (d *viaRuntimeDeath) Unwrap() error { return d.cause }

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
	return &viaBridgeDialConn{Conn: conn, cancelBridge: cancelBridge}, nil
}

// viaBridgeDialConn cancels an unclaimed bridge request when the corresponding
// frontend connection closes. It intentionally does not watch the dial context:
// connection-request contexts can end after a backend bridge is claimed.
type viaBridgeDialConn struct {
	net.Conn
	cancelBridge context.CancelFunc
}

func (c *viaBridgeDialConn) Close() error {
	c.cancelBridge()
	return c.Conn.Close()
}

type viaBridgeRequest struct {
	ctx    context.Context
	player Player
	cancel context.CancelFunc

	mu      sync.Mutex
	claimed bool
}

func (r *viaBridgeRequest) cancelIfUnclaimed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.claimed {
		r.cancel()
	}
}

func (r *viaBridgeRequest) claim() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil {
		return false
	}
	r.claimed = true
	return true
}

type viaBackendBridge struct {
	ln       net.Listener
	dialer   ServerDialer
	requests chan *viaBridgeRequest
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
		requests: make(chan *viaBridgeRequest, 1024),
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
	req := &viaBridgeRequest{ctx: streamCtx, player: player, cancel: cancel}
	select {
	case b.requests <- req:
		return req.cancelIfUnclaimed, nil
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
	var req *viaBridgeRequest
	for {
		select {
		case req = <-b.requests:
		case <-b.done:
			return
		}
		if req.claim() {
			break
		}
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

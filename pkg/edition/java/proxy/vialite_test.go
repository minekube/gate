//go:build !musl

package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	liteconfig "go.minekube.com/gate/pkg/edition/java/lite/config"
	vialite "go.minekube.com/vialite"
)

func TestViaServerInfoDialUsesTranslatedBackendAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
			close(accepted)
		}
	}()

	info := newViaServerInfo(
		NewServerInfo("lobby", mustParseAddr("127.0.0.1:25566")),
		&viaManagedRunner{server: &fakeVialiteServer{backends: map[string]string{"lobby": ln.Addr().String()}}},
	)
	dialer, ok := info.(ServerDialer)
	if !ok {
		t.Fatalf("ServerInfo = %T, want ServerDialer", info)
	}
	conn, err := dialer.Dial(context.Background(), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	_ = conn.Close()

	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("translated backend address was not dialed")
	}
}

func TestProxyInitWrapsViaConfigServer(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{"lobby": "127.0.0.1:25566"},
		Via: config.Via{
			Enabled: true,
		},
		Lite: liteconfig.Config{Enabled: false},
	}
	authenticator, err := auth.New(auth.Options{})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		authenticator: authenticator,
		via: &viaManagedRunner{
			cfg:            cfg,
			server:         &fakeVialiteServer{backends: map[string]string{"lobby": "127.0.0.1:25590"}},
			activeBackends: map[string]struct{}{"lobby": {}},
		},
	}

	if err := p.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	server := p.Server("lobby")
	if server == nil {
		t.Fatal("lobby server was not registered")
	}
	if _, ok := server.ServerInfo().(ServerDialer); !ok {
		t.Fatalf("ServerInfo = %T, want ServerDialer", server.ServerInfo())
	}
}

func TestProxyInitDoesNotWrapViaInLiteMode(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{"lobby": "127.0.0.1:25566"},
		Via: config.Via{
			Enabled: true,
		},
		Lite: liteconfig.Config{
			Enabled: true,
			Routes: []liteconfig.Route{{
				Host:    []string{"example.com"},
				Backend: []string{"127.0.0.1:25566"},
			}},
		},
	}
	authenticator, err := auth.New(auth.Options{})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		authenticator: authenticator,
		via: &viaManagedRunner{
			cfg:            cfg,
			server:         &fakeVialiteServer{backends: map[string]string{"lobby": "127.0.0.1:25590"}},
			activeBackends: map[string]struct{}{"lobby": {}},
		},
	}

	if err := p.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if server := p.Server("lobby"); server != nil {
		t.Fatalf("Lite mode registered classic server: %v", server)
	}
}

func TestProxyInitDoesNotWrapViaEnabledAfterStartup(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{"lobby": "127.0.0.1:25566"},
		Via: config.Via{
			Enabled: true,
		},
		Lite: liteconfig.Config{Enabled: false},
	}
	authenticator, err := auth.New(auth.Options{})
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		authenticator: authenticator,
		via:           &viaManagedRunner{cfg: cfg},
	}

	if err := p.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	server := p.Server("lobby")
	if server == nil {
		t.Fatal("lobby server was not registered")
	}
	if _, ok := server.ServerInfo().(ServerDialer); ok {
		t.Fatalf("ServerInfo = %T, did not want ServerDialer without active vialite", server.ServerInfo())
	}
}

func TestProxyRegisterAddsDynamicViaBackend(t *testing.T) {
	cfg := &config.Config{
		Forwarding: config.Forwarding{Mode: config.VelocityForwardingMode},
		Via:        config.Via{Enabled: true},
		Lite:       liteconfig.Config{Enabled: false},
	}
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		via: &viaManagedRunner{
			cfg:             cfg,
			server:          &fakeVialiteServer{backends: map[string]string{}},
			activeBackends:  map[string]struct{}{},
			dynamicBackends: map[string]*viaDynamicBackend{},
		},
	}

	server, err := p.Register(NewServerInfo("connect-session-1", mustParseAddr("127.0.0.1:25566")))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := server.ServerInfo().(ServerDialer); !ok {
		t.Fatalf("ServerInfo = %T, want ServerDialer", server.ServerInfo())
	}
	fake := p.via.server.(*fakeVialiteServer)
	added := fake.added["connect-session-1"]
	if added.Address != "127.0.0.1:25566" || added.Forwarding != vialite.ForwardingVelocity {
		t.Fatalf("unexpected added backend: %#v", added)
	}
}

func TestProxyUnregisterRemovesDynamicViaBackend(t *testing.T) {
	cfg := &config.Config{
		Via:  config.Via{Enabled: true},
		Lite: liteconfig.Config{Enabled: false},
	}
	fake := &fakeVialiteServer{backends: map[string]string{}}
	info := NewServerInfo("connect-session-1", mustParseAddr("127.0.0.1:25566"))
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		via: &viaManagedRunner{
			cfg:             cfg,
			server:          fake,
			activeBackends:  map[string]struct{}{},
			dynamicBackends: map[string]*viaDynamicBackend{},
		},
	}

	if _, err := p.Register(info); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !p.Unregister(info) {
		t.Fatal("Unregister returned false")
	}
	if !fake.removed["connect-session-1"] {
		t.Fatalf("dynamic backend was not removed: %#v", fake.removed)
	}
	if p.via.backendEnabled("connect-session-1") {
		t.Fatal("dynamic backend remained active after unregister")
	}
}

func TestProxyRegisterDynamicViaBackendPreservesServerDialer(t *testing.T) {
	cfg := &config.Config{
		Via:  config.Via{Enabled: true},
		Lite: liteconfig.Config{Enabled: false},
	}
	viaLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer viaLn.Close()
	fakeVia := &fakeVialiteServer{backends: map[string]string{}, viaAddr: viaLn.Addr().String()}
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		via: &viaManagedRunner{
			cfg:             cfg,
			server:          fakeVia,
			activeBackends:  map[string]struct{}{},
			dynamicBackends: map[string]*viaDynamicBackend{},
		},
	}
	original := &fakeDynamicDialer{
		name:        "connect-session-1",
		addr:        mustParseAddr("127.0.0.1:25566"),
		connections: make(chan net.Conn, 1),
		dialed:      make(chan Player, 1),
	}

	server, err := p.Register(original)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	added := fakeVia.added["connect-session-1"]
	if added.Address == original.Addr().String() {
		t.Fatalf("via backend address = %q, want bridge address", added.Address)
	}

	viaAccepted := make(chan struct{})
	go func() {
		viaConn, err := viaLn.Accept()
		if err != nil {
			return
		}
		defer viaConn.Close()
		bridgeConn, err := net.Dial("tcp", added.Address)
		if err != nil {
			return
		}
		defer bridgeConn.Close()
		close(viaAccepted)
		go func() { _, _ = io.Copy(bridgeConn, viaConn) }()
		_, _ = io.Copy(viaConn, bridgeConn)
	}()

	dialCtx, cancelDial := context.WithCancel(context.Background())
	dialer := server.ServerInfo().(ServerDialer)
	conn, err := dialer.Dial(dialCtx, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	cancelDial()
	select {
	case <-viaAccepted:
	case <-time.After(time.Second):
		t.Fatal("via listener was not dialed")
	}
	var backendConn net.Conn
	select {
	case backendConn = <-original.connections:
	case <-time.After(time.Second):
		t.Fatal("original ServerDialer was not used")
	}
	defer backendConn.Close()
	if _, err := conn.Write([]byte("x")); err != nil {
		t.Fatalf("write client: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := backendConn.Read(buf); err != nil {
		t.Fatalf("backend read: %v", err)
	}
	if string(buf) != "x" {
		t.Fatalf("backend read %q, want x", string(buf))
	}
	if _, err := backendConn.Write([]byte("y")); err != nil {
		t.Fatalf("backend write: %v", err)
	}
	if _, err := conn.Read(buf); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(buf) != "y" {
		t.Fatalf("client read %q, want y", string(buf))
	}
	select {
	case <-original.dialed:
	case <-time.After(time.Second):
		t.Fatal("original ServerDialer did not receive player context")
	}
}

func TestViaManagedRunnerStopClosesDynamicBridge(t *testing.T) {
	cfg := &config.Config{
		Via:  config.Via{Enabled: true},
		Lite: liteconfig.Config{Enabled: false},
	}
	fakeVia := &fakeVialiteServer{backends: map[string]string{}}
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		via: &viaManagedRunner{
			cfg:             cfg,
			server:          fakeVia,
			activeBackends:  map[string]struct{}{},
			dynamicBackends: map[string]*viaDynamicBackend{},
		},
	}
	original := &fakeDynamicDialer{
		name:        "connect-session-1",
		addr:        mustParseAddr("127.0.0.1:25566"),
		connections: make(chan net.Conn, 1),
		dialed:      make(chan Player, 1),
	}

	if _, err := p.Register(original); err != nil {
		t.Fatalf("Register: %v", err)
	}
	bridgeAddr := fakeVia.added["connect-session-1"].Address
	preStopConn, err := net.DialTimeout("tcp", bridgeAddr, time.Second)
	if err != nil {
		t.Fatalf("bridge not dialable before Stop: %v", err)
	}
	_ = preStopConn.Close()
	p.via.Stop()
	eventuallyDialFails(t, bridgeAddr)
}

func TestViaBackendBridgePrepareHonorsCallerCancelWhenQueueFull(t *testing.T) {
	bridge, err := newViaBackendBridge(&fakeDynamicDialer{
		name:        "connect-session-1",
		addr:        mustParseAddr("127.0.0.1:25566"),
		connections: make(chan net.Conn, 1),
		dialed:      make(chan Player, 1),
	})
	if err != nil {
		t.Fatalf("newViaBackendBridge: %v", err)
	}
	defer bridge.Close()
	for i := 0; i < cap(bridge.requests); i++ {
		cancel, err := bridge.Prepare(context.Background(), nil)
		if err != nil {
			t.Fatalf("fill request %d: %v", i, err)
		}
		defer cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := bridge.Prepare(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Prepare with full queue and canceled context = %v, want context.Canceled", err)
	}
}

func eventuallyDialFails(t *testing.T, addr string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		select {
		case <-deadline:
			t.Fatalf("address %s still dialable", addr)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestProxyRegisterFallsBackWhenDynamicViaUnsupported(t *testing.T) {
	cfg := &config.Config{
		Via:  config.Via{Enabled: true, Mode: "embedded"},
		Lite: liteconfig.Config{Enabled: false},
	}
	info := NewServerInfo("connect-session-1", mustParseAddr("127.0.0.1:25566"))
	p := &Proxy{
		log:           logr.Discard(),
		cfg:           cfg,
		event:         event.Nop,
		servers:       make(map[string]*registeredServer),
		configServers: make(map[string]bool),
		via: &viaManagedRunner{
			cfg:             cfg,
			server:          &fakeVialiteServer{backends: map[string]string{}, addErr: vialite.ErrDynamicBackendsUnsupported},
			activeBackends:  map[string]struct{}{},
			dynamicBackends: map[string]*viaDynamicBackend{},
		},
	}

	server, err := p.Register(info)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := server.ServerInfo().(*viaServerInfo); ok {
		t.Fatalf("ServerInfo = %T, did not want via wrapper when dynamic Via unsupported", server.ServerInfo())
	}
}

func TestViaManagedRunnerOptionsMapConfig(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{
			"lobby":    "127.0.0.1:25566",
			"survival": "127.0.0.1:25567",
		},
		Forwarding: config.Forwarding{Mode: config.VelocityForwardingMode},
		Via: config.Via{
			Enabled:     true,
			Mode:        "subprocess",
			Bind:        "127.0.0.1:25590",
			LibraryPath: "/opt/vialite/libvialite.so",
			BinaryPath:  "/opt/vialite/vialite",
			Version:     "v0.1.0",
			Mirror:      "https://mirror.example.com/vialite",
			Offline:     true,
		},
	}

	opts, err := newViaManagedRunner(cfg).options()
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if opts.Mode != vialite.ModeSubprocess {
		t.Fatalf("Mode = %v, want subprocess", opts.Mode)
	}
	if opts.Bind != "127.0.0.1:25590" || opts.LibraryPath != "/opt/vialite/libvialite.so" || opts.BinaryPath != "/opt/vialite/vialite" {
		t.Fatalf("unexpected paths/bind: %#v", opts)
	}
	if opts.Version != "v0.1.0" || opts.Mirror != "https://mirror.example.com/vialite" || !opts.Offline {
		t.Fatalf("unexpected release settings: %#v", opts)
	}
	if len(opts.Backends) != 2 {
		t.Fatalf("Backends len = %d, want 2", len(opts.Backends))
	}
	backendByName := map[string]vialite.Backend{}
	for _, backend := range opts.Backends {
		backendByName[backend.Name] = backend
	}
	for name, address := range cfg.Servers {
		backend, ok := backendByName[name]
		if !ok {
			t.Fatalf("missing backend %q in %#v", name, opts.Backends)
		}
		if backend.Address != address || backend.Forwarding != vialite.ForwardingVelocity {
			t.Fatalf("unexpected backend %q: %#v", name, backend)
		}
	}
}

func TestViaManagedRunnerOptionsAllowDynamicBackendsWithoutConfiguredServers(t *testing.T) {
	cfg := &config.Config{
		Via: config.Via{
			Enabled: true,
		},
	}

	opts, err := newViaManagedRunner(cfg).options()
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if !opts.AllowDynamicBackends {
		t.Fatal("AllowDynamicBackends = false")
	}
	if len(opts.Backends) != 0 {
		t.Fatalf("Backends len = %d, want 0", len(opts.Backends))
	}
}

func TestViaModeDefaults(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		goos        string
		libraryPath string
		want        vialite.Mode
	}{
		{name: "linux empty defaults subprocess", mode: "", goos: "linux", want: vialite.ModeSubprocess},
		{name: "linux explicit embedded", mode: "embedded", goos: "linux", want: vialite.ModeEmbedded},
		{name: "linux subprocess", mode: "subprocess", goos: "linux", want: vialite.ModeSubprocess},
		{name: "windows empty defaults subprocess", mode: "", goos: "windows", want: vialite.ModeSubprocess},
		{name: "windows explicit embedded uses subprocess", mode: "embedded", goos: "windows", want: vialite.ModeSubprocess},
		{name: "windows custom library explicit embedded", mode: "embedded", goos: "windows", libraryPath: "C:\\vialite\\libvialite.dll", want: vialite.ModeEmbedded},
		{name: "windows subprocess", mode: "subprocess", goos: "windows", want: vialite.ModeSubprocess},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := viaMode(tt.mode, tt.goos, tt.libraryPath); got != tt.want {
				t.Fatalf("viaMode(%q, %q, %q) = %v, want %v", tt.mode, tt.goos, tt.libraryPath, got, tt.want)
			}
		})
	}
}

func TestViaManagedRunnerOptionsUseConfiguredServerNames(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]string{
			"Lobby": "127.0.0.1:25566",
		},
		Via: config.Via{
			Enabled: true,
		},
	}

	opts, err := newViaManagedRunner(cfg).options()
	if err != nil {
		t.Fatalf("options: %v", err)
	}
	if len(opts.Backends) != 1 {
		t.Fatalf("Backends len = %d, want 1", len(opts.Backends))
	}
	backend := opts.Backends[0]
	if backend.Name != "Lobby" || backend.Address != "127.0.0.1:25566" {
		t.Fatalf("unexpected backend: %#v", backend)
	}
}

type fakeVialiteServer struct {
	backends map[string]string
	added    map[string]vialite.Backend
	removed  map[string]bool
	viaAddr  string
	addErr   error
}

func (f *fakeVialiteServer) Start(context.Context) error     { return nil }
func (f *fakeVialiteServer) WaitReady(context.Context) error { return nil }
func (f *fakeVialiteServer) Stop(context.Context) error      { return nil }
func (f *fakeVialiteServer) Healthy() bool                   { return true }

func (f *fakeVialiteServer) BackendDialAddress(name string) (string, error) {
	addr, ok := f.backends[name]
	if !ok {
		return "", errors.New("backend not found")
	}
	return addr, nil
}

func (f *fakeVialiteServer) AddBackend(ctx context.Context, backend vialite.Backend) (string, error) {
	if f.addErr != nil {
		return "", f.addErr
	}
	if f.backends == nil {
		f.backends = map[string]string{}
	}
	if f.added == nil {
		f.added = map[string]vialite.Backend{}
	}
	addr := f.viaAddr
	if addr == "" {
		addr = "127.0.0.1:25590"
	}
	f.added[backend.Name] = backend
	f.backends[backend.Name] = addr
	return addr, nil
}

func (f *fakeVialiteServer) RemoveBackend(ctx context.Context, name string) error {
	if f.removed == nil {
		f.removed = map[string]bool{}
	}
	f.removed[name] = true
	delete(f.backends, name)
	return nil
}

type fakeDynamicDialer struct {
	name        string
	addr        net.Addr
	connections chan net.Conn
	dialed      chan Player
}

func (f *fakeDynamicDialer) Name() string   { return f.name }
func (f *fakeDynamicDialer) Addr() net.Addr { return f.addr }

func (f *fakeDynamicDialer) Dial(ctx context.Context, player Player) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case f.dialed <- player:
	case <-ctx.Done():
		_ = client.Close()
		_ = server.Close()
		return nil, ctx.Err()
	}
	select {
	case f.connections <- server:
		return client, nil
	case <-ctx.Done():
		_ = client.Close()
		_ = server.Close()
		return nil, ctx.Err()
	}
}

//go:build !musl

package geyser

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	geyserlite "go.minekube.com/geyserlite"
)

// failFastBudget is the wall-clock budget in which Gate must report that the
// Bedrock UDP port is held by another process. A bind conflict is knowable
// before the native runtime is spawned, so anything near the ten-minute
// liteManagedStartupTimeout is a broken contract, not a slow machine.
const failFastBudget = 5 * time.Second

// holdUDPPort occupies a real UDP port on the wildcard address for the whole
// test. The failure under test is a genuine foreign listener on the port - it
// is reproduced, not mocked away.
func holdUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Fatalf("hold UDP port: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn.LocalAddr().(*net.UDPAddr).Port
}

// freeUDPPort returns a port that is free when this helper returns.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatalf("release UDP port: %v", err)
	}
	return port
}

func liteManagedTestConfig(t *testing.T, overrides map[string]any) *config.Config {
	t.Helper()
	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatalf("write floodgate key: %v", err)
	}
	return &config.Config{
		GeyserListenAddr: "localhost:25567",
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled:         true,
			Engine:          config.ManagedEngineGeyserlite,
			ConfigOverrides: overrides,
		},
	}
}

// TestLiteManagedRunnerFailsFastWhenBedrockPortIsHeld reproduces the shape
// measured in t_f872131c row x8: another process on the host (the operator's
// own Geyser) already holds the Bedrock UDP port the managed runtime needs.
// The runtime can never bind it and the RakNet front door never answers, but
// Gate used to stay up for the full ten-minute liteManagedStartupTimeout and
// only then stop the proxy, dropping every connected Java player.
func TestLiteManagedRunnerFailsFastWhenBedrockPortIsHeld(t *testing.T) {
	port := holdUDPPort(t)
	runner := newLiteManagedRunner(liteManagedTestConfig(t, map[string]any{
		"bedrock": map[string]any{"port": port},
	}))
	// A long backstop: passing must not come from the timeout path.
	runner.startupTimeout = 20 * time.Second

	served := make(chan struct{}, 1)
	runner.newServer = func(geyserlite.Options) (geyserliteServer, error) {
		served <- struct{}{}
		return &fakeGeyserliteServer{started: make(chan struct{})}, nil
	}

	begin := time.Now()
	err := runner.Start(context.Background())
	elapsed := time.Since(begin)

	if err == nil {
		t.Fatal("Start() error = nil, want a Bedrock port conflict error")
	}
	if elapsed > failFastBudget {
		t.Fatalf("Start() reported the port conflict after %s, want <= %s: %v", elapsed, failFastBudget, err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(port)) {
		t.Errorf("Start() error = %v, want it to name the held UDP port %d", err, port)
	}
	if !strings.Contains(err.Error(), "Address already in use") {
		t.Errorf("Start() error = %v, want the bind conflict string the support playbook greps for", err)
	}
	select {
	case <-served:
		t.Error("managed runtime was started although the Bedrock UDP port was already held")
	default:
	}

	var conflict *BedrockPortUnavailableError
	if !errors.As(err, &conflict) {
		t.Fatalf("Start() error = %#v, want a *BedrockPortUnavailableError", err)
	}
	if want := net.JoinHostPort("0.0.0.0", strconv.Itoa(port)); conflict.Addr != want {
		t.Errorf("conflict address = %q, want %q", conflict.Addr, want)
	}
	// The conflict must still wrap the real UDP bind failure, and it must keep
	// satisfying the platform classification predicate. Do not assert on
	// syscall.EADDRINUSE directly: on Windows that Go value is invented and a
	// real WinSock bind failure does not carry it.
	if !addrInUse(err) {
		t.Errorf("Start() error = %v, want it to unwrap to the platform bind conflict error", err)
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) || opErr.Op != "listen" || opErr.Net != "udp" {
		t.Errorf("Start() error = %v, want it to wrap the udp listen failure", err)
	}
}

// TestLiteManagedRunnerFailsFastForConfiguredBedrockAddress proves the probe
// checks the address the operator configured, including a non-wildcard bind.
func TestLiteManagedRunnerFailsFastForConfiguredBedrockAddress(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("hold UDP port: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	port := conn.LocalAddr().(*net.UDPAddr).Port

	runner := newLiteManagedRunner(liteManagedTestConfig(t, map[string]any{
		"bedrock": map[string]any{"address": "127.0.0.1", "port": port},
	}))
	runner.startupTimeout = 20 * time.Second

	err = runner.Start(context.Background())
	var conflict *BedrockPortUnavailableError
	if !errors.As(err, &conflict) {
		t.Fatalf("Start() error = %v, want a *BedrockPortUnavailableError", err)
	}
	if want := net.JoinHostPort("127.0.0.1", strconv.Itoa(port)); conflict.Addr != want {
		t.Errorf("conflict address = %q, want %q", conflict.Addr, want)
	}
}

// TestLiteManagedRunnerDoesNotFabricatePortConflicts guards the fail-closed
// boundary: only EADDRINUSE may be reported as a conflict. A bind failure for
// any other reason belongs to the runtime and its startup timeout, because a
// probe that over-triggers would stop a Gate that could have worked.
func TestLiteManagedRunnerDoesNotFabricatePortConflicts(t *testing.T) {
	runner := newLiteManagedRunner(liteManagedTestConfig(t, map[string]any{
		// TEST-NET-1 is not an address this host owns: binding it fails, but not
		// because somebody else holds the port.
		"bedrock": map[string]any{"address": "192.0.2.1", "port": freeUDPPort(t)},
	}))
	runner.startupTimeout = 5 * time.Second

	fake := &fakeGeyserliteServer{started: make(chan struct{})}
	runner.newServer = func(geyserlite.Options) (geyserliteServer, error) { return fake, nil }
	t.Cleanup(runner.Stop)

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Start(context.Background()) }()
	select {
	case <-fake.started:
	case err := <-errCh:
		t.Fatalf("Start() error = %v, want the runtime to be started: only EADDRINUSE may be classified as a conflict", err)
	case <-time.After(time.Second):
		t.Fatal("geyserlite server did not start")
	}
	fake.healthy.Store(true)
	if err := <-errCh; err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
}

// TestManagedBedrockListenAddrMatchesGeyserliteDerivation pins the address
// derivation to what geyserlite renders for the Bedrock listener: a wildcard
// :19132 baseline with Options.ConfigOverrides deep-merged over it last.
func TestManagedBedrockListenAddrMatchesGeyserliteDerivation(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		want      string
		wantOK    bool
	}{
		{name: "no overrides", want: "0.0.0.0:19132", wantOK: true},
		{name: "unrelated overrides", overrides: map[string]any{"debug-mode": true}, want: "0.0.0.0:19132", wantOK: true},
		{name: "yaml int port", overrides: map[string]any{"bedrock": map[string]any{"port": 19133}}, want: "0.0.0.0:19133", wantOK: true},
		{name: "json float port", overrides: map[string]any{"bedrock": map[string]any{"port": float64(19133)}}, want: "0.0.0.0:19133", wantOK: true},
		{name: "string port", overrides: map[string]any{"bedrock": map[string]any{"port": "19133"}}, want: "0.0.0.0:19133", wantOK: true},
		{
			name:      "address and port",
			overrides: map[string]any{"bedrock": map[string]any{"address": "127.0.0.1", "port": 12345}},
			want:      "127.0.0.1:12345",
			wantOK:    true,
		},
		{
			name:      "empty address keeps the wildcard",
			overrides: map[string]any{"bedrock": map[string]any{"address": ""}},
			want:      "0.0.0.0:19132",
			wantOK:    true,
		},
		{name: "unparseable port", overrides: map[string]any{"bedrock": map[string]any{"port": "not-a-port"}}, wantOK: false},
		{name: "out of range port", overrides: map[string]any{"bedrock": map[string]any{"port": 70000}}, wantOK: false},
		{name: "fractional port", overrides: map[string]any{"bedrock": map[string]any{"port": 19132.5}}, wantOK: false},
		{name: "unparseable address", overrides: map[string]any{"bedrock": map[string]any{"address": 127}}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := managedBedrockListenAddr(tt.overrides)
			if ok != tt.wantOK {
				t.Fatalf("managedBedrockListenAddr() ok = %t, want %t (addr %q)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Fatalf("managedBedrockListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLiteManagedRunnerIgnoresPortsItDoesNotBind guards the probe against
// over-triggering: a held port only matters when it is the port the managed
// runtime will actually bind.
func TestLiteManagedRunnerIgnoresPortsItDoesNotBind(t *testing.T) {
	held := holdUDPPort(t)
	runner := newLiteManagedRunner(liteManagedTestConfig(t, map[string]any{
		"bedrock": map[string]any{"port": freeUDPPort(t)},
	}))
	runner.startupTimeout = 5 * time.Second

	fake := &fakeGeyserliteServer{started: make(chan struct{})}
	runner.newServer = func(geyserlite.Options) (geyserliteServer, error) { return fake, nil }
	t.Cleanup(runner.Stop)

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Start(context.Background()) }()
	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatalf("geyserlite server did not start while port %d was held (only the configured port may be reasoned about)", held)
	}
	fake.healthy.Store(true)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start() error = %v, want nil for a free configured Bedrock port", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start() did not return after the runtime became healthy")
	}
}

// TestLiteManagedRunnerReleasesProbedBedrockPort proves the check does not
// itself occupy the port: the native runtime must still be able to bind it
// after the check ran.
func TestLiteManagedRunnerReleasesProbedBedrockPort(t *testing.T) {
	port := freeUDPPort(t)
	runner := newLiteManagedRunner(liteManagedTestConfig(t, map[string]any{
		"bedrock": map[string]any{"port": port},
	}))
	runner.startupTimeout = 5 * time.Second

	fake := &fakeGeyserliteServer{started: make(chan struct{})}
	runner.newServer = func(geyserlite.Options) (geyserliteServer, error) { return fake, nil }
	t.Cleanup(runner.Stop)

	errCh := make(chan error, 1)
	go func() { errCh <- runner.Start(context.Background()) }()
	select {
	case <-fake.started:
	case <-time.After(time.Second):
		t.Fatal("geyserlite server did not start")
	}
	fake.healthy.Store(true)
	if err := <-errCh; err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: port})
	if err != nil {
		t.Fatalf("bedrock port %d is still held after the check: %v", port, err)
	}
	_ = conn.Close()
}

// TestLiteManagedRunnerKeepsStartupTimeoutBackstop keeps the documented
// ten-minute path alive: a runtime that never becomes healthy for any other
// reason must still be reported as a startup timeout, not as a port conflict.
func TestLiteManagedRunnerKeepsStartupTimeoutBackstop(t *testing.T) {
	runner := newLiteManagedRunner(liteManagedTestConfig(t, map[string]any{
		"bedrock": map[string]any{"port": freeUDPPort(t)},
	}))
	runner.startupTimeout = 300 * time.Millisecond

	fake := &fakeGeyserliteServer{started: make(chan struct{})}
	runner.newServer = func(geyserlite.Options) (geyserliteServer, error) { return fake, nil }
	t.Cleanup(runner.Stop)

	err := runner.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want the startup timeout backstop error")
	}
	if !strings.Contains(err.Error(), "timed out after") ||
		!strings.Contains(err.Error(), "waiting for geyserlite to become healthy") {
		t.Fatalf("Start() error = %v, want the startup timeout backstop error", err)
	}
	if strings.Contains(err.Error(), "Address already in use") {
		t.Fatalf("Start() error = %v, want a timeout for a free port, not a port conflict", err)
	}
	if liteManagedStartupTimeout != 10*time.Minute {
		t.Errorf("liteManagedStartupTimeout = %s, want 10m: the support playbook greps the ten-minute path", liteManagedStartupTimeout)
	}
}

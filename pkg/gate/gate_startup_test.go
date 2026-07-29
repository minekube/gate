package gate

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/robinbraemer/event"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"go.minekube.com/gate/pkg/configs"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/config"
)

// Startup smoke tests: they boot Gate the same way `gate` does (config file ->
// LoadConfig -> New/Start) and assert it initializes cleanly, so a regression
// that breaks booting fails CI instead of users.
//
// Everything here is hermetic: no external network, no real Minecraft clients.
// Listen addresses are reserved free localhost ports and Connect never talks to
// the public Connect network.

const (
	startupTimeout  = 30 * time.Second
	shutdownTimeout = 30 * time.Second
)

// TestStartupShippedConfigs boots Gate with every config template that ships
// with `gate config -t <type>` and asserts a clean startup: the config parses
// and validates, all components wire up, the proxy listener binds and Gate
// shuts down without error.
func TestStartupShippedConfigs(t *testing.T) {
	tests := []struct {
		name   string
		config []byte
		verify func(t *testing.T, g *Gate)
	}{
		{name: "minimal", config: configs.MinimalConfigBytes},
		{name: "simple", config: configs.SimpleConfigBytes},
		{
			name:   "full",
			config: configs.DefaultConfigBytes,
			verify: func(t *testing.T, g *Gate) {
				require.True(t, g.Java().Config().Wasm.Enabled,
					"the shipped full config must enable the built-in Wasm plugin")
			},
		},
		{
			name:   "lite",
			config: configs.LiteConfigBytes,
			verify: func(t *testing.T, g *Gate) {
				// Guard that this case really booted the Gate Lite path.
				lite := g.Java().Config().Lite
				require.True(t, lite.Enabled, "Gate must run in Lite mode")
				require.NotEmpty(t, lite.Routes, "Lite routes must be loaded from the config")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadTestConfig(t, tt.config)
			cfg.Config.Bind = reserveAddr(t)

			g := bootGate(t, cfg)
			requireDialable(t, cfg.Config.Bind)
			if tt.verify != nil {
				tt.verify(t, g.Gate)
			}
		})
	}
}

// TestStartupTopLevelStart boots Gate through the Start entrypoint used by
// cmd/gate, including OpenTelemetry init and the config file watcher, and
// asserts it comes up and shuts down without error.
//
// Start creates its own event manager, so readiness is observed by dialing the
// proxy listener instead of subscribing to jproxy.ReadyEvent.
func TestStartupTopLevelStart(t *testing.T) {
	// Keep otelutil.Init from exporting to an inherited OTLP endpoint.
	t.Setenv("OTEL_TRACES_ENABLED", "false")
	t.Setenv("OTEL_METRICS_ENABLED", "false")

	configFile := writeConfigFile(t, configs.SimpleConfigBytes)
	v := viper.New()
	v.SetConfigFile(configFile)
	cfg, err := LoadConfig(v)
	require.NoError(t, err, "shipped config must load")
	cfg.Config.Bind = reserveAddr(t)

	log, logs := testLogger(t)
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), log))
	defer cancel()

	startResult := make(chan error, 1)
	go func() {
		startResult <- Start(ctx,
			WithConfig(*cfg),
			WithAutoShutdownOnSignal(false),
			WithAutoConfigReload(configFile),
		)
	}()

	deadline := time.Now().Add(startupTimeout)
	for !dialable(cfg.Config.Bind) {
		select {
		case err := <-startResult:
			t.Fatalf("Gate exited during startup: %v\n%s", err, logs)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("Gate did not listen on %s within %s\n%s", cfg.Config.Bind, startupTimeout, logs)
		}
		time.Sleep(25 * time.Millisecond)
	}
	require.True(t, logs.waitFor("listening for connections", startupTimeout),
		"Gate must log that it listens for connections")

	cancel()
	select {
	case err := <-startResult:
		require.NoError(t, err, "Gate must shut down without error")
	case <-time.After(shutdownTimeout):
		t.Fatal("Gate did not shut down")
	}
}

// TestStartupConnectAndAPI boots Gate with Connect and the HTTP API enabled and
// asserts everything comes up, so a broken Connect or API bootstrap fails here.
//
// The self-hosted Connect service is exercised end to end (it binds a local
// port). The Connect client is exercised up to the point where it dials the
// watch service: it is pointed at an unreachable local address instead of the
// public Connect network, which also asserts that an unreachable watch service
// does not take Gate down.
func TestStartupConnectAndAPI(t *testing.T) {
	// Keep the token handling hermetic against a developer's environment.
	t.Setenv("CONNECT_TOKEN", "")

	tokenFile := filepath.Join(t.TempDir(), "connect.json")

	cfg := loadTestConfig(t, configs.MinimalConfigBytes)
	cfg.Config.Bind = reserveAddr(t)
	cfg.Connect.Enabled = true
	cfg.Connect.Service.Enabled = true
	cfg.Connect.Service.Addr = reserveAddr(t)
	cfg.Connect.WatchServiceAddr = "ws://" + reserveAddr(t) + "/watch"
	// A name must be set, otherwise Gate fetches a random one over the internet.
	cfg.Connect.Name = "gate-startup-smoke-test"
	cfg.Connect.TokenFilePath = tokenFile
	cfg.API.Enabled = true
	cfg.API.Config.Bind = reserveAddr(t)

	g := bootGate(t, cfg)
	requireDialable(t, cfg.Config.Bind)
	requireDialable(t, cfg.Connect.Service.Addr)
	requireDialable(t, cfg.API.Config.Bind)
	require.True(t, g.logs.waitFor("Connect service started", startupTimeout),
		"self-hosted Connect service must start")
	require.True(t, g.logs.waitFor("connecting to watch service", startupTimeout),
		"Connect client must bootstrap and dial the watch service")
	require.FileExists(t, tokenFile, "Connect client must provision its token file")
}

// TestStartupBedrockConfigWiring asserts the Bedrock config template parses,
// validates and wires up a Bedrock proxy.
//
// Limitation: Bedrock startup itself is not exercised because the Bedrock proxy
// runs a managed Geyser process (downloading a jar and requiring a JVM), which
// is neither hermetic nor fast. See the geyserlite smoke test in .github/workflows/ci.yml
// for the coverage of that path.
func TestStartupBedrockConfigWiring(t *testing.T) {
	cfg := loadTestConfig(t, configs.BedrockConfigBytes)
	cfg.Config.Bind = reserveAddr(t)
	require.True(t, cfg.Config.Bedrock.Enabled, "bedrock config template must enable Bedrock")

	g, err := New(Options{Config: cfg, EventMgr: event.New()})
	require.NoError(t, err, "Gate must wire up the Bedrock proxy")
	require.NotNil(t, g.Bedrock(), "Bedrock proxy must be created")
	require.NotNil(t, g.Java(), "Java proxy must be created")
}

// TestStartupRejectsInvalidConfig guards the smoke tests above: a Gate that
// cannot be configured must fail to start instead of booting half-initialized.
func TestStartupRejectsInvalidConfig(t *testing.T) {
	_, err := New(Options{})
	require.Error(t, err, "Gate must not start without a config")

	cfg := loadTestConfig(t, configs.MinimalConfigBytes)
	cfg.Config.Bind = "not-a-host-port"
	_, err = New(Options{Config: cfg})
	require.Error(t, err, "Gate must not start with an invalid bind address")
}

// bootedGate is a running Gate instance owned by a test.
type bootedGate struct {
	*Gate
	logs *logRecorder
}

// bootGate starts Gate with cfg and blocks until the proxy listener is ready.
// The Gate is stopped on test cleanup and its startup must not return an error.
func bootGate(t *testing.T, cfg *config.Config) *bootedGate {
	t.Helper()

	events := event.New(event.WithRecoverPanic(false))
	ready := make(chan string, 1)
	unsubscribe := event.Subscribe(events, 0, func(e *jproxy.ReadyEvent) {
		select {
		case ready <- e.Addr():
		default:
		}
	})
	t.Cleanup(unsubscribe)

	g, err := New(Options{Config: cfg, EventMgr: events})
	require.NoError(t, err, "Gate must wire up all components of a valid config")

	log, logs := testLogger(t)
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), log))
	startResult := make(chan error, 1)
	go func() { startResult <- g.Start(ctx) }()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-startResult:
			require.NoError(t, err, "Gate must shut down without error")
		case <-time.After(shutdownTimeout):
			t.Error("Gate did not shut down")
		}
	})

	select {
	case addr := <-ready:
		require.Equal(t, cfg.Config.Bind, addr, "Gate must listen on the configured bind address")
	case err := <-startResult:
		startResult <- nil // cleanup already waits on this channel
		t.Fatalf("Gate exited during startup: %v\n%s", err, logs)
	case <-time.After(startupTimeout):
		t.Fatalf("Gate did not become ready within %s\n%s", startupTimeout, logs)
	}

	return &bootedGate{Gate: g, logs: logs}
}

// loadTestConfig writes the config to a temp file and loads it the same way the
// gate command does, so config parsing is part of what the test covers.
func loadTestConfig(t *testing.T, config []byte) *config.Config {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(writeConfigFile(t, config))
	cfg, err := LoadConfig(v)
	require.NoError(t, err, "config must load")
	return cfg
}

func writeConfigFile(t *testing.T, config []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, config, 0o600))
	return path
}

// reserveAddr returns a free localhost address to bind on.
func reserveAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// requireDialable asserts addr accepts connections within the startup timeout.
// Components started in the background may bind slightly after Gate is ready.
func requireDialable(t *testing.T, addr string) {
	t.Helper()
	require.Eventually(t, func() bool { return dialable(addr) },
		startupTimeout, 25*time.Millisecond, "nothing is listening on %s", addr)
}

func dialable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// testLogger returns a logger recording Gate's startup logs. They are only
// printed if the test fails, so a CI failure shows why startup went wrong.
func testLogger(t *testing.T) (logr.Logger, *logRecorder) {
	rec := new(logRecorder)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Gate startup logs:\n%s", rec)
		}
	})
	return funcr.New(func(prefix, args string) {
		rec.add(strings.TrimSpace(prefix + " " + args))
	}, funcr.Options{Verbosity: 1}), rec
}

// logRecorder collects log lines from Gate's goroutines. It deliberately does
// not write to testing.T, which would panic when a lingering goroutine logs
// after the test finished.
type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *logRecorder) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
}

func (r *logRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

// waitFor reports whether a log line containing sub was recorded within timeout.
func (r *logRecorder) waitFor(sub string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if strings.Contains(r.String(), sub) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

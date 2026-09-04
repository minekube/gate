//go:build geyserlite_gate_acceptance && linux && amd64 && !musl

package geyser

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	geyserlite "go.minekube.com/geyserlite"
)

// TestGateManagedGeyserliteAutoDownloadColdAndWarmCacheRakNet proves the
// supported Gate-managed subprocess path, not just geyserlite in isolation:
// Gate maps its managed config into Options, waits for Healthy, and a real
// released native asset returns a RakNet unconnected-pong. The warm run uses
// the exact cache entry left by the cold run; its mirror refuses binary
// requests, so a re-download is a failure.
//
// It is deliberately opt-in. It downloads a public release asset and starts a
// native Geyser process, so ordinary unit tests must stay hermetic and quick.
func TestGateManagedGeyserliteAutoDownloadColdAndWarmCacheRakNet(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("GEYSERLITE_BINARY", "")
	t.Setenv("GEYSERLITE_LIBRARY", "")
	// Do not let a local installation make the cold-cache proof pass through a
	// PATH lookup. The test helper itself is invoked by absolute path.
	t.Setenv("PATH", "/usr/bin:/bin")

	runGateManagedAcceptance(t, cacheHome, "", 5*time.Minute)
	asset, checksum := gateManagedCachedBinary(t, cacheHome)

	var binaryRequests atomic.Int32
	warmMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + geyserlite.DefaultVersion + "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", checksum, filepath.Base(asset))
		default:
			binaryRequests.Add(1)
			http.Error(w, "warm cache must not download the binary", http.StatusGone)
		}
	}))
	defer warmMirror.Close()

	runGateManagedAcceptance(t, cacheHome, warmMirror.URL, 2*time.Minute)
	if got := binaryRequests.Load(); got != 0 {
		t.Fatalf("warm Gate-managed run requested a binary %d times", got)
	}
}

// TestGateManagedGeyserliteAcceptanceHelper has to run in a fresh test process
// for each cache state: the native Geyser runtime owns process-global state.
func TestGateManagedGeyserliteAcceptanceHelper(t *testing.T) {
	if os.Getenv("GATE_GEYSERLITE_ACCEPTANCE_HELPER") != "1" {
		return
	}

	listen := os.Getenv("GATE_GEYSERLITE_ACCEPTANCE_LISTEN")
	upstream := os.Getenv("GATE_GEYSERLITE_ACCEPTANCE_UPSTREAM")
	phasePath := os.Getenv("GATE_GEYSERLITE_ACCEPTANCE_PHASES")
	if listen == "" || upstream == "" || phasePath == "" {
		os.Exit(2)
	}
	_, portText, err := net.SplitHostPort(listen)
	if err != nil {
		os.Exit(2)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		os.Exit(2)
	}

	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef"), 0o600); err != nil {
		os.Exit(1)
	}

	phaseHandler := newGateManagedAcceptancePhaseHandler(phasePath)
	slog.SetDefault(slog.New(phaseHandler))
	runner := newLiteManagedRunner(&config.Config{
		GeyserListenAddr: upstream,
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled: true,
			Engine:  config.ManagedEngineGeyserlite,
			Mode:    "subprocess",
			Mirror:  os.Getenv("GATE_GEYSERLITE_ACCEPTANCE_MIRROR"),
			ConfigOverrides: map[string]any{
				"bedrock": map[string]any{
					"address": "127.0.0.1",
					"port":    port,
				},
			},
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runner.Start(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			runner.Stop()
			return
		}
		os.Exit(1)
	}
	phaseHandler.markGateRunnerHealthy()
	<-ctx.Done()
	runner.Stop()
}

func runGateManagedAcceptance(t *testing.T, cacheHome, mirror string, budget time.Duration) {
	t.Helper()
	listen := gateManagedReserveUDPAddr(t)
	upstream := gateManagedReserveTCPAddr(t)
	phasePath := filepath.Join(t.TempDir(), "phases.json")

	cmd := exec.Command(os.Args[0], "-test.run=^TestGateManagedGeyserliteAcceptanceHelper$")
	cmd.Env = append(os.Environ(),
		"GATE_GEYSERLITE_ACCEPTANCE_HELPER=1",
		"GATE_GEYSERLITE_ACCEPTANCE_LISTEN="+listen,
		"GATE_GEYSERLITE_ACCEPTANCE_UPSTREAM="+upstream,
		"GATE_GEYSERLITE_ACCEPTANCE_MIRROR="+mirror,
		"GATE_GEYSERLITE_ACCEPTANCE_PHASES="+phasePath,
		"XDG_CACHE_HOME="+cacheHome,
		"GEYSERLITE_BINARY=",
		"GEYSERLITE_LIBRARY=",
		"PATH=/usr/bin:/bin",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s Gate-managed acceptance child: %v", gateManagedAcceptancePhase(mirror), err)
	}

	probeCtx, cancelProbe := context.WithTimeout(context.Background(), budget)
	reached := gateManagedWaitForRakNetStatus(probeCtx, listen)
	cancelProbe()
	if !reached {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
		phases := readGateManagedAcceptancePhases(phasePath)
		phases.CacheBinaryPresent = gateManagedCacheBinaryPresent(cacheHome)
		encoded, _ := json.Marshal(phases)
		t.Fatalf("%s Gate-managed cache did not reach a RakNet status reply within %s; phases=%s", gateManagedAcceptancePhase(mirror), budget, encoded)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("stop %s Gate-managed acceptance child: %v", gateManagedAcceptancePhase(mirror), err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s Gate-managed acceptance child stopped uncleanly: %v", gateManagedAcceptancePhase(mirror), err)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("%s Gate-managed acceptance child did not stop within 30s", gateManagedAcceptancePhase(mirror))
	}
}

// gateManagedAcceptancePhases deliberately contains only fixed, low-cardinality
// state. In particular it never persists Geyser log lines, errors, paths,
// usernames, XUIDs, player addresses, or packet data.
type gateManagedAcceptancePhases struct {
	LocatedBinary      bool `json:"located_binary"`
	SubprocessStarted  bool `json:"subprocess_started"`
	NativeReadyLog     bool `json:"native_ready_log"`
	RestartObserved    bool `json:"restart_observed"`
	GateRunnerHealthy  bool `json:"gate_runner_healthy"`
	CacheBinaryPresent bool `json:"cache_binary_present"`
}

type gateManagedAcceptancePhaseHandler struct {
	state *gateManagedAcceptancePhaseState
}

type gateManagedAcceptancePhaseState struct {
	mu   sync.Mutex
	path string
	data gateManagedAcceptancePhases
}

func newGateManagedAcceptancePhaseLogger(path string) *slog.Logger {
	return slog.New(newGateManagedAcceptancePhaseHandler(path))
}

func newGateManagedAcceptancePhaseHandler(path string) *gateManagedAcceptancePhaseHandler {
	return &gateManagedAcceptancePhaseHandler{state: &gateManagedAcceptancePhaseState{path: path}}
}

func (h *gateManagedAcceptancePhaseHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *gateManagedAcceptancePhaseHandler) Handle(_ context.Context, record slog.Record) error {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	changed := false
	switch record.Message {
	case "located geyserlite binary":
		changed = !h.state.data.LocatedBinary
		h.state.data.LocatedBinary = true
	case "started geyserlite subprocess":
		changed = !h.state.data.SubprocessStarted
		h.state.data.SubprocessStarted = true
	case "geyser exited with error; restarting after backoff":
		changed = !h.state.data.RestartObserved
		h.state.data.RestartObserved = true
	default:
		if gateManagedNativeReady(record.Message) {
			changed = !h.state.data.NativeReadyLog
			h.state.data.NativeReadyLog = true
		}
	}
	if changed {
		gateManagedWriteAcceptancePhasesLocked(h.state.path, h.state.data)
	}
	return nil
}

func (h *gateManagedAcceptancePhaseHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *gateManagedAcceptancePhaseHandler) WithGroup(string) slog.Handler      { return h }

func (h *gateManagedAcceptancePhaseHandler) markGateRunnerHealthy() {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	if h.state.data.GateRunnerHealthy {
		return
	}
	h.state.data.GateRunnerHealthy = true
	gateManagedWriteAcceptancePhasesLocked(h.state.path, h.state.data)
}

func gateManagedNativeReady(message string) bool {
	// Keep this diagnostic in lockstep with geyserlite's Healthy marker. The
	// native logger may prefix a line, but only this fixed boolean is retained.
	return strings.Contains(message, "Done (")
}

func gateManagedWriteAcceptancePhasesLocked(path string, phases gateManagedAcceptancePhases) {
	data, err := json.Marshal(phases)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

func readGateManagedAcceptancePhases(path string) gateManagedAcceptancePhases {
	data, err := os.ReadFile(path)
	if err != nil {
		return gateManagedAcceptancePhases{}
	}
	var phases gateManagedAcceptancePhases
	if json.Unmarshal(data, &phases) != nil {
		return gateManagedAcceptancePhases{}
	}
	return phases
}

func TestGateManagedAcceptancePhaseLoggerEmitsOnlyFixedBooleans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phases.json")
	logger := newGateManagedAcceptancePhaseLogger(path)
	logger.Info("located geyserlite binary", slog.String("path", "/private/path"))
	logger.Info("started geyserlite subprocess", slog.Int("pid", 12345))
	logger.Warn("private native error must be ignored")
	logger.Info("Done (1.23s)!", slog.String("player", "private-player"))

	got := readGateManagedAcceptancePhases(path)
	want := gateManagedAcceptancePhases{LocatedBinary: true, SubprocessStarted: true, NativeReadyLog: true}
	if got != want {
		t.Fatalf("phase flags = %+v, want %+v", got, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private", "12345", "player", "error", "path"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("phase file contains forbidden free-form data %q", forbidden)
		}
	}
}

func gateManagedAcceptancePhase(mirror string) string {
	if mirror == "" {
		return "cold"
	}
	return "warm"
}

func gateManagedReserveUDPAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("reserve Bedrock UDP port: %v", err)
	}
	addr := conn.LocalAddr().String()
	if err := conn.Close(); err != nil {
		t.Fatalf("release Bedrock UDP port: %v", err)
	}
	return addr
}

func gateManagedReserveTCPAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Java upstream port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release Java upstream port: %v", err)
	}
	return addr
}

func gateManagedCachedBinary(t *testing.T, cacheHome string) (string, string) {
	t.Helper()
	root := filepath.Join(cacheHome, "geyserlite", geyserlite.DefaultVersion)
	var asset, checksum string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "geyserlite-linux-amd64" {
			return nil
		}
		candidate := filepath.Base(filepath.Dir(path))
		if len(candidate) != 64 || strings.Trim(candidate, "0123456789abcdef") != "" {
			return fmt.Errorf("cache binary is not under a sha256 directory")
		}
		asset, checksum = path, candidate
		return filepath.SkipAll
	})
	if err != nil {
		t.Fatalf("inspect cold Gate-managed cache: %v", err)
	}
	if asset == "" || checksum == "" {
		t.Fatal("cold Gate-managed start did not leave a content-addressed geyserlite binary cache entry")
	}
	return asset, checksum
}

func gateManagedCacheBinaryPresent(cacheHome string) bool {
	root := filepath.Join(cacheHome, "geyserlite", geyserlite.DefaultVersion)
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() == "geyserlite-linux-amd64" {
			parent := filepath.Base(filepath.Dir(path))
			if len(parent) == 64 && strings.Trim(parent, "0123456789abcdef") == "" {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

func gateManagedWaitForRakNetStatus(ctx context.Context, addr string) bool {
	for {
		if gateManagedRakNetStatus(ctx, addr) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// gateManagedRakNetStatus is intentionally a tiny unconnected-ping probe. It
// validates a genuine Bedrock/RakNet status reply without authenticating a
// player or retaining the server descriptor.
func gateManagedRakNetStatus(ctx context.Context, addr string) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	conn, err := (&net.Dialer{Timeout: time.Until(deadline)}).DialContext(ctx, "udp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	ping := make([]byte, 33)
	ping[0] = 0x01 // RakNet Unconnected Ping.
	binary.BigEndian.PutUint64(ping[1:9], uint64(time.Now().UnixMilli()))
	copy(ping[9:25], gateManagedRakNetMagic[:])
	binary.BigEndian.PutUint64(ping[25:33], 1)
	if _, err := conn.Write(ping); err != nil {
		return false
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil || n < 35 || buf[0] != 0x1c { // RakNet Unconnected Pong.
		return false
	}
	if !gateManagedEqualBytes(buf[17:33], gateManagedRakNetMagic[:]) {
		return false
	}
	descriptorLength := int(binary.BigEndian.Uint16(buf[33:35]))
	if 35+descriptorLength > n {
		return false
	}
	// MCPE is the protocol-level marker for the real Bedrock status response.
	return strings.HasPrefix(string(buf[35:35+descriptorLength]), "MCPE;")
}

var gateManagedRakNetMagic = [...]byte{
	0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe,
	0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78,
}

func gateManagedEqualBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

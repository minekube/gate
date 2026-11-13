package gate

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"
)

// bedrockPortConflictStartupBudget bounds how long Gate may take to report a
// Bedrock UDP port conflict. The condition is knowable before the managed
// runtime is spawned, so it must never be spent on the ten-minute managed
// startup timeout (t_f872131c row x8 measured the proxy staying up for
// 10m0s and then dropping every Java player).
const bedrockPortConflictStartupBudget = 20 * time.Second

// TestStartupBedrockPortConflictFailsFast boots Gate the way `gate` does on a
// host where another process already holds the Bedrock UDP port, which is the
// measured production shape of the reported "nobody can join, any edition"
// symptom: the managed runtime can never bind, the RakNet front door never
// answers, and Gate used to only notice after ten minutes - taking the Java
// listener and every connected Java player down with it.
//
// The failure is reproduced for real: a UDP socket holds the port for the
// whole test, and Gate is booted from a parsed config template with Bedrock
// enabled. Because the conflict is detected before the native runtime is
// spawned, this stays hermetic - no Geyser download and no JVM.
func TestStartupBedrockPortConflictFailsFast(t *testing.T) {
	// Keep a stray GeyserLite cache/download out of the developer's home; the
	// conflict is detected before any download, so this stays hermetic.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	holder, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err, "hold the Bedrock UDP port")
	t.Cleanup(func() { _ = holder.Close() })
	heldPort := holder.LocalAddr().(*net.UDPAddr).Port

	keyPath := filepath.Join(t.TempDir(), "floodgate.pem")
	cfg := loadTestConfig(t, []byte(fmt.Sprintf(`
config:
  bind: 127.0.0.1:25565
  onlineMode: true
  servers:
    server1: 127.0.0.1:25566
  try:
    - server1
  bedrock:
    enabled: true
    geyserListenAddr: %s
    floodgateKeyPath: %s
    managed:
      enabled: true
      engine: geyserlite
      configOverrides:
        bedrock:
          address: 127.0.0.1
          port: %d
`, reserveAddr(t), keyPath, heldPort)))
	cfg.Config.Bind = reserveAddr(t)
	require.True(t, cfg.Config.Bedrock.Enabled, "the test config must really enable Bedrock")

	g, err := New(Options{Config: cfg, EventMgr: event.New()})
	require.NoError(t, err, "Gate must wire up the Bedrock proxy")

	log, logs := testLogger(t)
	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), log))
	defer cancel()

	startResult := make(chan error, 1)
	begin := time.Now()
	go func() { startResult <- g.Start(ctx) }()

	select {
	case err := <-startResult:
		elapsed := time.Since(begin)
		require.Error(t, err, "Gate must not start with a foreign listener on the Bedrock UDP port")
		require.LessOrEqual(t, elapsed, bedrockPortConflictStartupBudget,
			"Gate took %s to report the Bedrock port conflict", elapsed)
		require.Contains(t, err.Error(), strconv.Itoa(heldPort),
			"the error must name the held Bedrock UDP port")
		require.Contains(t, err.Error(), "Address already in use",
			"the error must carry the bind conflict string the support playbook greps for")
		require.NotContains(t, logs.String(), "timed out after 10m0s",
			"the conflict must not be reported through the ten-minute startup timeout")
		require.True(t, logs.waitFor("Address already in use", 5*time.Second),
			"Gate must log the actionable port conflict, not just return it:\n%s", logs)
	case <-time.After(bedrockPortConflictStartupBudget):
		t.Fatalf("Gate did not report the held Bedrock UDP port %d within %s - a Bedrock-only port conflict "+
			"still costs the Java listener and every connected player:\n%s",
			heldPort, bedrockPortConflictStartupBudget, logs)
	}

	// Whichever Java listener Gate managed to open must be gone: the failure is
	// immediate and loud instead of ten minutes of a silently degraded proxy.
	require.Eventually(t, func() bool { return !dialable(cfg.Config.Bind) },
		bedrockPortConflictStartupBudget, 25*time.Millisecond,
		"the Java listener on %s survived the reported Bedrock failure", cfg.Config.Bind)
}

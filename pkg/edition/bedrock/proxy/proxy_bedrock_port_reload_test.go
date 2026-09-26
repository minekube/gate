package proxy

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/robinbraemer/event"

	"go.minekube.com/gate/pkg/edition/bedrock/config"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
)

// TestConfigUpdateWithHeldBedrockPortFailsFast covers the reload entry point of
// the managed Bedrock runtime. A config update that restarts the integration
// onto a Bedrock UDP port another process holds used to sit in the managed
// startup wait and only then stop the proxy; it must now report the conflict
// immediately, so the Java listener is not silently degraded meanwhile.
func TestConfigUpdateWithHeldBedrockPortFailsFast(t *testing.T) {
	holder, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Fatalf("hold Bedrock UDP port: %v", err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	heldPort := holder.LocalAddr().(*net.UDPAddr).Port

	freeHolder, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero})
	if err != nil {
		t.Fatalf("reserve free Bedrock UDP port: %v", err)
	}
	freePort := freeHolder.LocalAddr().(*net.UDPAddr).Port
	_ = freeHolder.Close()

	keyPath := filepath.Join(t.TempDir(), "floodgate.key")
	if err := os.WriteFile(keyPath, bytes.Repeat([]byte{0x24}, 16), 0o600); err != nil {
		t.Fatalf("write floodgate key: %v", err)
	}

	javaProxy, err := jproxy.New(jproxy.Options{Config: &jconfig.DefaultConfig, EventMgr: event.Nop})
	if err != nil {
		t.Fatalf("jproxy.New() error = %v", err)
	}

	previous := &config.Config{
		GeyserListenAddr: "127.0.0.1:0",
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled:         true,
			Engine:          config.ManagedEngineGeyserlite,
			ConfigOverrides: map[string]any{"bedrock": map[string]any{"port": freePort}},
		},
	}
	updated := &config.Config{
		GeyserListenAddr: "127.0.0.1:0",
		FloodgateKeyPath: keyPath,
		Managed: &config.ManagedGeyser{
			Enabled:         true,
			Engine:          config.ManagedEngineGeyserlite,
			ConfigOverrides: map[string]any{"bedrock": map[string]any{"port": heldPort}},
		},
	}
	if !requiresRestart(previous, updated) {
		t.Fatal("requiresRestart() = false, but the Bedrock port override change must restart the integration")
	}

	p := &Proxy{
		config:          previous,
		javaProxy:       javaProxy,
		runtimeFailures: make(chan error, 1),
	}

	begin := time.Now()
	p.handleConfigUpdate(context.Background(), &bedrockConfigUpdateEvent{
		PrevConfig: previous,
		Config:     updated,
	})

	select {
	case err := <-p.runtimeFailures:
		elapsed := time.Since(begin)
		if elapsed > 5*time.Second {
			t.Fatalf("reload reported the Bedrock port conflict after %s: %v", elapsed, err)
		}
		if !containsAll(err.Error(), "Address already in use", strconv.Itoa(heldPort)) {
			t.Fatalf("reload error = %v, want the bind conflict naming UDP port %d", err, heldPort)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reload did not report the held Bedrock UDP port within 5s")
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if !bytes.Contains([]byte(haystack), []byte(needle)) {
			return false
		}
	}
	return true
}

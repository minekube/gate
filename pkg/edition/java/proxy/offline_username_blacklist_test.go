package proxy

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/gate/proto"
)

func TestOfflineModeUsernameBlacklist(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}

	// all is the default, so direct and Connect offline sessions retain the
	// original protection behaviour.
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, false, "adminname"))
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, true, "adminname"))
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, false, "AdminName"),
		"ForceOnline must stay authenticated even if the listener defaults offline")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, true, "AdminName"),
		"Connect ingress must not override ForceOnline")
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName"),
		"the proxy-wide online-mode default must remain authenticated")

	cfg.OnlineMode = false
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "ADMINNAME"))
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "ADMINNAME"))
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AnotherPlayer"))
}

func TestOfflineModeUsernameBlacklistConnectScope(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OnlineMode = false
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}
	cfg.OfflineModeUsernameBlacklistScope = config.OfflineModeUsernameBlacklistScopeConnect

	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName"),
		"a direct offline join must not be classified from its hostname, IP, or handshake")
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName"),
		"only the authenticated Connect tunnel provenance marker enables this scope")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, true, "AdminName"),
		"Mojang-authenticated joins remain exempt")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, false, "AdminName"))

	// A Connect session explicitly forced offline is still an offline session.
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, true, "AdminName"))
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, false, "AdminName"))
}

func TestOfflineModeUsernameBlacklistScopeEmptyIsCompatible(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OnlineMode = false
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}
	cfg.OfflineModeUsernameBlacklistScope = ""

	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName"),
		"an omitted key in an older config must continue to mean all")
}

func TestOfflineModeUsernameBlacklistConcurrentReads(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OnlineMode = false
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}
	cfg.OfflineModeUsernameBlacklistScope = config.OfflineModeUsernameBlacklistScopeConnect

	var wg sync.WaitGroup
	errs := make(chan bool, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(connectIngress bool) {
			defer wg.Done()
			got := offlineModeUsernameBlocked(&cfg, AllowedPreLogin, connectIngress, "AdminName")
			errs <- got == connectIngress
		}(i%2 == 0)
	}
	wg.Wait()
	close(errs)
	for ok := range errs {
		require.True(t, ok)
	}
}

type markedConnectTunnelConn struct{ net.Conn }

func (markedConnectTunnelConn) IsConnectTunnelIngress() bool { return true }

func TestConnectTunnelIngressUsesAdapterMarker(t *testing.T) {
	directServer, directClient := net.Pipe()
	t.Cleanup(func() { _ = directServer.Close() })
	t.Cleanup(func() { _ = directClient.Close() })
	direct, _ := netmc.NewMinecraftConn(context.Background(), directServer, proto.ServerBound, time.Second, time.Second, -1, nil)
	require.False(t, connectTunnelIngress(direct), "a direct socket has no Connect provenance")

	tunnelServer, tunnelClient := net.Pipe()
	t.Cleanup(func() { _ = tunnelServer.Close() })
	t.Cleanup(func() { _ = tunnelClient.Close() })
	tunnel, _ := netmc.NewMinecraftConn(context.Background(), markedConnectTunnelConn{tunnelServer}, proto.ServerBound, time.Second, time.Second, -1, nil)
	require.True(t, connectTunnelIngress(tunnel), "only a trusted adapter marker reaches login through netmc")
}

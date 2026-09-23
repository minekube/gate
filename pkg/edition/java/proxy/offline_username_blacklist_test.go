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
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestOfflineModeUsernameBlacklist(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}

	// all is the default, so direct and Connect offline sessions retain the
	// original protection behaviour.
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, false, "adminname", nil))
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, true, "adminname", nil))
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, false, "AdminName", nil),
		"ForceOnline must stay authenticated even if the listener defaults offline")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, true, "AdminName", nil),
		"Connect ingress must not override ForceOnline")
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName", nil),
		"the proxy-wide online-mode default must remain authenticated")

	cfg.OnlineMode = false
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "ADMINNAME", nil))
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "ADMINNAME", nil))
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AnotherPlayer", nil))
}

func TestOfflineModeUsernameBlacklistConnectScope(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OnlineMode = false
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}
	cfg.OfflineModeUsernameBlacklistScope = config.OfflineModeUsernameBlacklistScopeConnect

	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName", nil),
		"a direct offline join must not be classified from its hostname, IP, or handshake")
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName", nil),
		"only the authenticated Connect tunnel provenance marker enables this scope")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, true, "AdminName", nil),
		"Mojang-authenticated joins remain exempt")
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, false, "AdminName", nil))

	// A Connect session explicitly forced offline is still an offline session.
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, true, "AdminName", nil))
	require.False(t, offlineModeUsernameBlocked(&cfg, ForceOfflineModePreLogin, false, "AdminName", nil))
}

// TestOfflineModeUsernameBlacklistOfflineIdentity pins the #1113 determination:
// a session whose identity is an unauthenticated offline identity is a login
// path that is effectively offline mode even when the proxy runs online-mode,
// and an authenticated identity is exempt however it was obtained.
func TestOfflineModeUsernameBlacklistOfflineIdentity(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}
	cfg.OfflineModeUsernameBlacklistScope = config.OfflineModeUsernameBlacklistScopeConnect

	offlineIdentity := profile.NewOffline("AdminName")

	// The reporter's configuration: online-mode proxy, scope connect.
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName", offlineIdentity),
		"a Connect tunnel's offline identity is an offline login path")
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName",
		&profile.GameProfile{ID: uuid.New(), Name: "AdminName"}),
		"a Connect-verified identity must keep the reserved name")
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName", nil),
		"no supplied identity means the join still has to authenticate")

	// The list match stays case-insensitive, and the identity comparison must not
	// become fail-open when a login path re-cases the claimed name.
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "ADMINNAME", offlineIdentity))
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AnotherName", profile.NewOffline("AnotherName")))

	// Any identity carrying an offline UUID digest is unauthenticated, so it is
	// an offline login path even when the identity's own name is not the claimed
	// one (fail-closed: never let an unauthenticated identity claim a reserved
	// name through a renaming/rewriting login path).
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName", profile.NewOffline("SomeoneElse")))

	// An authenticated identity (random UUID) keeps the name, even when the
	// claimed login name differs in case or the profile carries no name.
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "ADMINNAME",
		&profile.GameProfile{ID: uuid.New(), Name: "AdminName"}))
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, true, "AdminName",
		&profile.GameProfile{ID: uuid.New()}))

	// Fail-closed: forcing online mode does not authenticate an identity that
	// arrives with the offline UUID — login would still complete with it.
	require.True(t, offlineModeUsernameBlocked(&cfg, ForceOnlineModePreLogin, true, "AdminName", offlineIdentity))

	// The scope still decides which ingress the reservation covers.
	require.False(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName", offlineIdentity),
		"scope connect excludes a non-Connect ingress even for an offline identity")

	cfg.OfflineModeUsernameBlacklistScope = config.OfflineModeUsernameBlacklistScopeAll
	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName", offlineIdentity))
}

func TestOfflineModeUsernameBlacklistScopeEmptyIsCompatible(t *testing.T) {
	cfg := config.DefaultConfig
	cfg.OnlineMode = false
	cfg.OfflineModeUsernameBlacklist = []string{"AdminName"}
	cfg.OfflineModeUsernameBlacklistScope = ""

	require.True(t, offlineModeUsernameBlocked(&cfg, AllowedPreLogin, false, "AdminName", nil),
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
			got := offlineModeUsernameBlocked(&cfg, AllowedPreLogin, connectIngress, "AdminName", nil)
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

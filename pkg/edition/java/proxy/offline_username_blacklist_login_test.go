package proxy

import (
	"bytes"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/funcr"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/configutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

const (
	blacklistReservedName = "ReservedAdmin"
	blacklistOtherName    = "RegularPlayer"
	// blacklistReasonText is deliberately not the shipped default so the tests
	// prove the *configured* reason is what the player receives.
	blacklistReasonText = "reserved-name-test-reason"
)

// blacklistIngressConn models the connection Gate's Connect tunnel adapter
// hands to Proxy.handleConn: the trusted ingress marker plus the game profile
// the TunnelService authenticated (or, for an offline/cracked session, the
// offline identity it derived for the requested name).
type blacklistIngressConn struct {
	net.Conn
	gp *profile.GameProfile
}

func (c *blacklistIngressConn) GameProfile() *profile.GameProfile { return c.gp }
func (c *blacklistIngressConn) IsConnectTunnelIngress() bool      { return true }

// blacklistProfileConn carries a game profile without Connect provenance —
// what a plugin attaching an identity to a direct connection looks like. The
// blacklist scope must keep treating it as a non-Connect ingress.
type blacklistProfileConn struct {
	net.Conn
	gp *profile.GameProfile
}

func (c *blacklistProfileConn) GameProfile() *profile.GameProfile { return c.gp }

// blacklistLoginOutcome is what the client observed during one login attempt.
type blacklistLoginOutcome struct {
	kicked            bool
	kickPayload       []byte
	loginSuccess      bool
	encryptionRequest bool
}

// runBlacklistLogin drives one real login through Proxy.handleConn and reports
// how it ended. wrap() decorates the server side of the client connection with
// the ingress identity (nil means a plain direct connection).
func runBlacklistLogin(
	t *testing.T,
	cfg config.Config,
	wrap func(net.Conn) net.Conn,
	username string,
) blacklistLoginOutcome {
	t.Helper()

	cfg.Bind = "127.0.0.1:0"
	cfg.Forwarding.Mode = config.NoneForwardingMode
	cfg.Compression.Threshold = -1 // no SetCompression frame in the wire assertions
	cfg.Servers = map[string]string{"lobby": "127.0.0.1:1"}
	cfg.Try = []string{"lobby"}

	p, err := New(Options{Config: &cfg})
	if err != nil {
		t.Fatalf("proxy New error: %v", err)
	}
	// Proxy goroutines can outlive the test body, so stop logging with it.
	var testDone atomic.Bool
	defer testDone.Store(true)
	p.log = funcr.New(func(prefix, args string) {
		if !testDone.Load() {
			t.Logf("PROXY: %s %s", prefix, args)
		}
	}, funcr.Options{Verbosity: 1})
	if err := p.init(); err != nil {
		t.Fatalf("proxy init error: %v", err)
	}

	client, server := net.Pipe()
	conn := net.Conn(server)
	if wrap != nil {
		conn = wrap(server)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.HandleConn(conn)
	}()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("HandleConn did not return after the client connection was closed")
		}
	})

	if err := writeTunnelHandshake(client, "gilly-smp.minekube.net", 25565, int(version.Minecraft_1_20.Protocol)); err != nil {
		t.Fatalf("client: failed to send handshake: %v", err)
	}
	if err := writeServerLogin(client, username); err != nil {
		t.Fatalf("client: failed to send ServerLogin: %v", err)
	}

	var outcome blacklistLoginOutcome
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = client.SetReadDeadline(deadline)
		packetID, data, err := readPacket(client)
		if err != nil {
			break
		}
		switch packetID {
		case 0x00: // login-state Disconnect (kick)
			outcome.kicked = true
			outcome.kickPayload = data
		case 0x01: // login-state EncryptionRequest
			outcome.encryptionRequest = true
		case 0x02: // login-state LoginSuccess
			outcome.loginSuccess = true
		}
		if outcome.kicked || outcome.loginSuccess || outcome.encryptionRequest {
			break
		}
	}
	return outcome
}

// TestOfflineModeUsernameBlacklistLoginPath is the wire-level reproduction of
// minekube/gate#1113: reserved-name protection was derived from the proxy's own
// onlineMode, so on a normal `onlineMode: true` proxy an offline/cracked player
// arriving through a Connect tunnel completed login under a reserved name
// (offline UUID, no kick) even with `offlineModeUsernameBlacklistScope: connect`.
//
// The matrix covers both ingress kinds (direct vs Connect) and both proxy modes,
// and pins the contract the config documents: an unauthenticated offline
// identity is blocked when the name is reserved, an authenticated join under the
// same name is not.
func TestOfflineModeUsernameBlacklistLoginPath(t *testing.T) {
	const (
		identityNone    = "none"    // no game profile supplied (direct connection)
		identityOffline = "offline" // unauthenticated offline identity for the name
		identityPremium = "premium" // authenticated identity (random v4 UUID)
	)

	tests := []struct {
		name                  string
		onlineMode            bool
		scope                 config.OfflineModeUsernameBlacklistScope
		connectIngress        bool
		identity              string
		username              string
		wantKick              bool
		wantLoginSuccess      bool
		wantEncryptionRequest bool
	}{
		{
			name:           "connect offline identity, online-mode proxy, scope connect, reserved",
			onlineMode:     true,
			scope:          config.OfflineModeUsernameBlacklistScopeConnect,
			connectIngress: true,
			identity:       identityOffline,
			username:       blacklistReservedName,
			wantKick:       true,
		},
		{
			name:             "connect authenticated identity, online-mode proxy, scope connect, reserved",
			onlineMode:       true,
			scope:            config.OfflineModeUsernameBlacklistScopeConnect,
			connectIngress:   true,
			identity:         identityPremium,
			username:         blacklistReservedName,
			wantLoginSuccess: true,
		},
		{
			name:           "connect offline identity, online-mode proxy, scope all, reserved",
			onlineMode:     true,
			scope:          config.OfflineModeUsernameBlacklistScopeAll,
			connectIngress: true,
			identity:       identityOffline,
			username:       blacklistReservedName,
			wantKick:       true,
		},
		{
			name:                  "direct connection, online-mode proxy, scope connect, reserved",
			onlineMode:            true,
			scope:                 config.OfflineModeUsernameBlacklistScopeConnect,
			identity:              identityNone,
			username:              blacklistReservedName,
			wantEncryptionRequest: true,
		},
		{
			name:           "connect offline identity, offline-mode proxy, scope connect, reserved",
			onlineMode:     false,
			scope:          config.OfflineModeUsernameBlacklistScopeConnect,
			connectIngress: true,
			identity:       identityOffline,
			username:       blacklistReservedName,
			wantKick:       true,
		},
		{
			name:             "direct connection, offline-mode proxy, scope connect, reserved",
			onlineMode:       false,
			scope:            config.OfflineModeUsernameBlacklistScopeConnect,
			identity:         identityNone,
			username:         blacklistReservedName,
			wantLoginSuccess: true,
		},
		{
			name:       "direct connection, offline-mode proxy, scope all, reserved",
			onlineMode: false,
			scope:      config.OfflineModeUsernameBlacklistScopeAll,
			identity:   identityNone,
			username:   blacklistReservedName,
			wantKick:   true,
		},
		{
			name:             "connect offline identity, online-mode proxy, scope connect, unreserved name",
			onlineMode:       true,
			scope:            config.OfflineModeUsernameBlacklistScopeConnect,
			connectIngress:   true,
			identity:         identityOffline,
			username:         blacklistOtherName,
			wantLoginSuccess: true,
		},
		{
			name:             "direct connection with plugin-supplied offline identity, online-mode proxy, scope connect",
			onlineMode:       true,
			scope:            config.OfflineModeUsernameBlacklistScopeConnect,
			identity:         identityOffline,
			username:         blacklistReservedName,
			wantLoginSuccess: true,
		},
		{
			name:       "direct connection with plugin-supplied offline identity, online-mode proxy, scope all, reserved",
			onlineMode: true,
			scope:      config.OfflineModeUsernameBlacklistScopeAll,
			identity:   identityOffline,
			username:   blacklistReservedName,
			wantKick:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig
			cfg.OnlineMode = tc.onlineMode
			cfg.OfflineModeUsernameBlacklist = []string{blacklistReservedName}
			cfg.OfflineModeUsernameBlacklistScope = tc.scope
			cfg.OfflineModeUsernameBlacklistReason = &configutil.TextComponent{Content: blacklistReasonText}

			var wrap func(net.Conn) net.Conn
			switch tc.identity {
			case identityOffline:
				gp := profile.NewOffline(tc.username)
				if tc.connectIngress {
					wrap = func(c net.Conn) net.Conn { return &blacklistIngressConn{Conn: c, gp: gp} }
				} else {
					wrap = func(c net.Conn) net.Conn { return &blacklistProfileConn{Conn: c, gp: gp} }
				}
			case identityPremium:
				gp := &profile.GameProfile{ID: uuid.New(), Name: tc.username}
				if tc.connectIngress {
					wrap = func(c net.Conn) net.Conn { return &blacklistIngressConn{Conn: c, gp: gp} }
				} else {
					wrap = func(c net.Conn) net.Conn { return &blacklistProfileConn{Conn: c, gp: gp} }
				}
			}

			outcome := runBlacklistLogin(t, cfg, wrap, tc.username)

			if tc.wantKick {
				if !outcome.kicked {
					t.Fatalf("reserved name %q was not kicked (loginSuccess=%v encryptionRequest=%v); "+
						"the offline-identity login path was not recognised as effectively offline "+
						"(minekube/gate#1113)", tc.username, outcome.loginSuccess, outcome.encryptionRequest)
				}
				if outcome.loginSuccess {
					t.Fatal("the session was both kicked and completed login")
				}
				if !bytes.Contains(outcome.kickPayload, []byte(blacklistReasonText)) {
					t.Fatalf("kick payload %q does not carry the configured reason %q", outcome.kickPayload, blacklistReasonText)
				}
				return
			}

			if outcome.kicked {
				t.Fatalf("kicked with %q, want no blacklist kick for this login path", outcome.kickPayload)
			}
			if tc.wantLoginSuccess && !outcome.loginSuccess {
				t.Fatalf("login did not complete (encryptionRequest=%v)", outcome.encryptionRequest)
			}
			if tc.wantEncryptionRequest && !outcome.encryptionRequest {
				t.Fatalf("expected an online-mode EncryptionRequest (loginSuccess=%v)", outcome.loginSuccess)
			}
		})
	}
}

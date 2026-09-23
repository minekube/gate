package proxy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

// duplicateLoginConfig returns a config that only displaces duplicate logins
// through the identity store, with the older onlineMode setting out of the way.
func duplicateLoginConfig(storeEnabled, kickExisting bool) *config.Config {
	cfg := config.DefaultConfig
	cfg.OnlineModeKickExistingPlayers = false
	cfg.IdentityStore.Enabled = storeEnabled
	cfg.IdentityStore.KickExistingPlayers = kickExisting
	return &cfg
}

// kickablePlayer returns a player whose disconnect unregisters it, the way a
// real session tears down synchronously inside netmc's Close.
func kickablePlayer(p *Proxy, name string) *connectedPlayer {
	player := &connectedPlayer{profile: &profile.GameProfile{ID: uuid.New(), Name: name}}
	ctx, cancel := context.WithCancel(context.Background())
	player.MinecraftConn = &closingTestConn{
		testMinecraftConn: &testMinecraftConn{ctx: ctx},
		onClose: func() {
			cancel()
			p.unregisterConnection(player)
		},
	}
	return player
}

type closingTestConn struct {
	*testMinecraftConn
	onClose func()
}

func (c *closingTestConn) Close() error {
	if c.onClose != nil {
		c.onClose()
	}
	return nil
}

// With identityStore.kickExistingPlayers the new login displaces the player who
// is already online under that username, even though the two logins resolved to
// different UUIDs (offline and authenticated).
func TestIdentityKickExistingPlayersDisplacesOlderSession(t *testing.T) {
	cfg := duplicateLoginConfig(true, true)
	p := &Proxy{
		cfg:         cfg,
		playerNames: map[string]*connectedPlayer{},
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
	}

	older := kickablePlayer(p, "Steve")
	p.registerConnection(older)

	newer := kickablePlayer(p, "sTeVe") // the registry keys names case-insensitively
	if !p.canRegisterConnection(newer) {
		t.Fatal("canRegisterConnection() = false, want true for a duplicate login")
	}
	if !p.registerConnection(newer) {
		t.Fatal("registerConnection() = false, want true")
	}

	if !older.disconnectDueToDuplicateConnection.Load() {
		t.Fatal("the displaced session was not marked as a duplicate login")
	}
	if got := p.playerNames["steve"]; got != newer {
		t.Fatalf("playerNames[steve] = %v, want the new session", got)
	}
	if got := p.playerIDs[newer.ID()]; got != newer {
		t.Fatalf("playerIDs[new] = %v, want the new session", got)
	}
	if _, ok := p.playerIDs[older.ID()]; ok {
		t.Fatal("the displaced session is still registered by UUID")
	}
}

// The same setting without the store must not kick: without it the two logins
// are different accounts, so the duplicate is rejected as before.
func TestIdentityKickExistingPlayersRequiresStore(t *testing.T) {
	cfg := duplicateLoginConfig(false, true)
	p := &Proxy{
		cfg:         cfg,
		playerNames: map[string]*connectedPlayer{},
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
	}
	older := kickablePlayer(p, "Steve")
	p.registerConnection(older)

	newer := kickablePlayer(p, "Steve")
	if p.canRegisterConnection(newer) {
		t.Fatal("canRegisterConnection() = true, want false while the store is disabled")
	}
	if p.registerConnection(newer) {
		t.Fatal("registerConnection() = true, want false for a duplicate login")
	}
	if !older.Active() {
		t.Fatal("the already online player was disconnected although the store is disabled")
	}
}

// Enabled but not kicking keeps the previous behaviour: the second login is
// rejected and the player who is online stays.
func TestIdentityStoreWithoutKickRejectsDuplicate(t *testing.T) {
	cfg := duplicateLoginConfig(true, false)
	p := &Proxy{
		cfg:         cfg,
		playerNames: map[string]*connectedPlayer{},
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
	}
	older := kickablePlayer(p, "Steve")
	p.registerConnection(older)

	newer := kickablePlayer(p, "Steve")
	if p.canRegisterConnection(newer) {
		t.Fatal("canRegisterConnection() = true, want false without kickExistingPlayers")
	}
	if p.registerConnection(newer) {
		t.Fatal("registerConnection() = true, want false without kickExistingPlayers")
	}
	if !older.Active() {
		t.Fatal("the already online player was disconnected without kickExistingPlayers")
	}
}

// A session that never unregisters must not spin the registry forever.
func TestRegisterConnectionGivesUpOnStuckDuplicate(t *testing.T) {
	cfg := duplicateLoginConfig(true, true)
	p := &Proxy{
		cfg:         cfg,
		playerNames: map[string]*connectedPlayer{},
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
	}
	// Disconnect does not unregister this one: it stays in the maps.
	stuck := &connectedPlayer{
		profile:       &profile.GameProfile{ID: uuid.New(), Name: "Steve"},
		MinecraftConn: &testMinecraftConn{},
	}
	p.registerConnection(stuck)

	newer := kickablePlayer(p, "Steve")
	if p.registerConnection(newer) {
		t.Fatal("registerConnection() = true, want false for a stuck duplicate")
	}
	if got := p.playerNames["steve"]; got != stuck {
		t.Fatalf("playerNames[Steve] = %v, want the stuck session to stay registered", got)
	}
}

func TestRegisterConnectionUnlocksAfterDuplicate(t *testing.T) {
	tests := map[string]struct {
		existing  *connectedPlayer
		candidate *connectedPlayer
	}{
		"username": {
			existing:  &connectedPlayer{profile: &profile.GameProfile{ID: uuid.New(), Name: "Player"}},
			candidate: &connectedPlayer{profile: &profile.GameProfile{ID: uuid.New(), Name: "player"}},
		},
		"id": {
			existing:  &connectedPlayer{profile: &profile.GameProfile{ID: uuid.New(), Name: "first"}},
			candidate: &connectedPlayer{profile: &profile.GameProfile{Name: "second"}},
		},
	}
	tests["id"].candidate.profile.ID = tests["id"].existing.profile.ID

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := config.DefaultConfig
			cfg.OnlineModeKickExistingPlayers = false
			p := &Proxy{
				cfg:         &cfg,
				playerNames: map[string]*connectedPlayer{"player": test.existing, "first": test.existing},
				playerIDs:   map[uuid.UUID]*connectedPlayer{test.existing.ID(): test.existing},
			}

			if p.registerConnection(test.candidate) {
				t.Fatal("registerConnection() = true, want false for duplicate player")
			}
			if !p.muP.TryLock() {
				t.Fatal("registerConnection() returned with player mutex locked")
			}
			p.muP.Unlock()
		})
	}
}

// A login that was rejected as a duplicate still tears down through
// unregisterConnection, and with the identity store it resolves to the very
// same UUID the player who is online holds. Unregistering the rejected login
// must therefore not remove that player's entries: doing so used to take the
// online player out of the registry, which is why the duplicate login only
// succeeded on the second attempt.
func TestUnregisterRejectedDuplicateKeepsOnlinePlayer(t *testing.T) {
	for name, candidateID := range map[string]func(stored uuid.UUID) uuid.UUID{
		"same uuid as the online player (identity store)": func(id uuid.UUID) uuid.UUID { return id },
		"different uuid (no identity store)":              func(uuid.UUID) uuid.UUID { return uuid.New() },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := duplicateLoginConfig(true, false) // duplicate logins are rejected
			p := &Proxy{
				cfg:         cfg,
				playerNames: map[string]*connectedPlayer{},
				playerIDs:   map[uuid.UUID]*connectedPlayer{},
			}

			online := kickablePlayer(p, "Steve")
			if !p.registerConnection(online) {
				t.Fatal("registerConnection() = false, want true for the first login")
			}

			rejected := &connectedPlayer{profile: &profile.GameProfile{ID: candidateID(online.ID()), Name: "Steve"}}
			require.False(t, p.canRegisterConnection(rejected),
				"the duplicate login must still be rejected while kickExistingPlayers is off")

			if p.unregisterConnection(rejected) {
				t.Fatal("unregisterConnection() = true, want false for a player that never registered")
			}
			if got := p.playerNames["steve"]; got != online {
				t.Fatalf("playerNames[steve] = %v, want the online player to stay registered", got)
			}
			if got := p.playerIDs[online.ID()]; got != online {
				t.Fatalf("playerIDs[online] = %v, want the online player to stay registered", got)
			}
		})
	}
}

// A player's own unregistration still removes both keys.
func TestUnregisterRemovesOwnEntries(t *testing.T) {
	cfg := duplicateLoginConfig(true, false)
	p := &Proxy{
		cfg:         cfg,
		playerNames: map[string]*connectedPlayer{},
		playerIDs:   map[uuid.UUID]*connectedPlayer{},
	}
	player := kickablePlayer(p, "Steve")
	p.registerConnection(player)

	if !p.unregisterConnection(player) {
		t.Fatal("unregisterConnection() = false, want true for a registered player")
	}
	if _, ok := p.playerNames["steve"]; ok {
		t.Fatal("playerNames[steve] still holds the unregistered player")
	}
	if _, ok := p.playerIDs[player.ID()]; ok {
		t.Fatal("playerIDs still holds the unregistered player")
	}
}

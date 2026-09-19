package proxy

import (
	"testing"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

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

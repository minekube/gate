package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

// The UUID a player finally has on a backend server is the one Gate forwards
// with player info forwarding enabled, and the one the backend derives from the
// username when forwarding is disabled. The same account therefore has a
// different UUID on the backend depending on the forwarding and online-mode
// configuration, which is what the login log reports as "backendUid".
func TestBackendPlayerID(t *testing.T) {
	const username = "Alice"
	profileID := uuid.New() // a Mojang-authenticated or plugin-provided profile UUID
	player := &connectedPlayer{
		profile: &profile.GameProfile{ID: profileID, Name: username},
	}

	for _, mode := range []config.ForwardingMode{
		config.LegacyForwardingMode,
		config.VelocityForwardingMode,
		config.BungeeGuardForwardingMode,
	} {
		require.Equal(t, profileID, backendPlayerID(mode, player),
			"%s forwarding forwards the profile UUID to the backend", mode)
	}

	require.Equal(t, uuid.OfflinePlayerUUID(username), backendPlayerID(config.NoneForwardingMode, player),
		"without forwarding the backend derives the offline UUID from the username")
}

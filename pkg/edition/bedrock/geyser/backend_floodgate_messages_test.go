package geyser

import (
	"context"
	"reflect"
	"testing"

	"github.com/robinbraemer/event"
	"github.com/stretchr/testify/require"

	bedrockconfig "go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	jconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
	jproxy "go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

var _ jproxy.ClientPluginMessageFilter = (*Integration)(nil)

type filterTestPlayer struct {
	jproxy.Player
	ctx         context.Context
	gameProfile profile.GameProfile
}

func (p filterTestPlayer) Context() context.Context         { return p.ctx }
func (p filterTestPlayer) GameProfile() profile.GameProfile { return p.gameProfile }

func filterTestProxy(t *testing.T, mode jconfig.ForwardingMode) *jproxy.Proxy {
	t.Helper()
	cfg := jconfig.DefaultConfig
	cfg.Forwarding.Mode = mode
	p, err := jproxy.New(jproxy.Options{Config: &cfg, EventMgr: event.Nop})
	require.NoError(t, err)
	return p
}

func bedrockFilterPlayer(t *testing.T, mode jconfig.ForwardingMode, data *floodgate.BedrockData, profileID uuid.UUID) jproxy.Player {
	t.Helper()
	ctx := withBedrockContext(context.Background(), &GeyserConnection{BedrockData: data})
	return filterTestPlayer{
		ctx:         ctx,
		gameProfile: profile.GameProfile{ID: profileID},
	}
}

func backendFloodgateIntegration(t *testing.T, mode jconfig.ForwardingMode, enabled bool, allowed ...string) *Integration {
	t.Helper()
	return &Integration{
		proxy: filterTestProxy(t, mode),
		config: &bedrockconfig.Config{
			BackendFloodgate: bedrockconfig.BackendFloodgate{
				Enabled:        enabled,
				AllowedServers: allowed,
			},
		},
	}
}

func xuidIdentity(t *testing.T, data *floodgate.BedrockData) uuid.UUID {
	t.Helper()
	id, err := data.JavaUuid()
	require.NoError(t, err)
	return id
}

// The regression for the backend Floodgate kick: an unlinked Bedrock player
// reaches the backend under Gate's XUID-derived v5 UUID, which a backend
// Floodgate never derives and cannot map to a FloodgatePlayer. Forwarding a
// client Floodgate message for that session is answered by a kick
// ("Only Floodgate players can send floodgate messages!"), which is what Geyser
// triggers one second after spawn when it uploads the player's skin.
func TestClientPluginMessageDroppedForUnresolvableBackendFloodgateSession(t *testing.T) {
	data := testBedrockData()
	i := backendFloodgateIntegration(t, jconfig.VelocityForwardingMode, true, "lobby")
	player := bedrockFilterPlayer(t, jconfig.VelocityForwardingMode, data, xuidIdentity(t, data))

	for _, channel := range []string{
		floodgate.SkinChannel,
		floodgate.FormChannel,
		floodgate.TransferChannel,
		floodgate.PacketChannel,
	} {
		require.False(t, i.AllowClientPluginMessage(channel, player, testBackendTarget("lobby")),
			"channel %q must be withheld from a backend Floodgate that cannot resolve the session", channel)
	}
}

// A linked (promoted) player reaches the backend under the linked Java UUID,
// which the backend Floodgate resolves by getCorrectUniqueId - so its plugin
// messages must keep flowing (form responses, skin uploads).
func TestClientPluginMessageKeptForLinkedBackendFloodgateSession(t *testing.T) {
	data := testBedrockData()
	i := backendFloodgateIntegration(t, jconfig.VelocityForwardingMode, true, "lobby")
	linkedID, err := uuid.Parse("c66dfcbc-4bd2-4a29-8c76-eadf80faa08a")
	require.NoError(t, err)
	player := bedrockFilterPlayer(t, jconfig.VelocityForwardingMode, data, linkedID)

	require.True(t, i.AllowClientPluginMessage(floodgate.SkinChannel, player, testBackendTarget("lobby")))
	require.True(t, i.AllowClientPluginMessage(floodgate.FormChannel, player, testBackendTarget("lobby")))
}

// Without Velocity forwarding the backend Floodgate assigns the player UUID
// itself, so it resolves every session whatever identity Gate picked.
func TestClientPluginMessageKeptWithoutVelocityForwarding(t *testing.T) {
	data := testBedrockData()
	i := backendFloodgateIntegration(t, jconfig.NoneForwardingMode, true, "lobby")
	player := bedrockFilterPlayer(t, jconfig.NoneForwardingMode, data, xuidIdentity(t, data))

	require.True(t, i.AllowClientPluginMessage(floodgate.SkinChannel, player, testBackendTarget("lobby")))
}

func TestClientPluginMessageKeptOutsideBackendFloodgateScope(t *testing.T) {
	data := testBedrockData()
	unlinked := xuidIdentity(t, data)

	t.Run("backend Floodgate disabled", func(t *testing.T) {
		i := backendFloodgateIntegration(t, jconfig.VelocityForwardingMode, false, "lobby")
		player := bedrockFilterPlayer(t, jconfig.VelocityForwardingMode, data, unlinked)
		require.True(t, i.AllowClientPluginMessage(floodgate.SkinChannel, player, testBackendTarget("lobby")))
	})

	t.Run("target not allowlisted", func(t *testing.T) {
		i := backendFloodgateIntegration(t, jconfig.VelocityForwardingMode, true, "lobby")
		player := bedrockFilterPlayer(t, jconfig.VelocityForwardingMode, data, unlinked)
		require.True(t, i.AllowClientPluginMessage(floodgate.SkinChannel, player, testBackendTarget("survival")))
	})

	t.Run("channel is not Floodgate's", func(t *testing.T) {
		i := backendFloodgateIntegration(t, jconfig.VelocityForwardingMode, true, "lobby")
		player := bedrockFilterPlayer(t, jconfig.VelocityForwardingMode, data, unlinked)
		require.True(t, i.AllowClientPluginMessage("minecraft:brand", player, testBackendTarget("lobby")))
		require.True(t, i.AllowClientPluginMessage("floodgate", player, testBackendTarget("lobby")))
	})

	t.Run("session is not from Geyser", func(t *testing.T) {
		i := backendFloodgateIntegration(t, jconfig.VelocityForwardingMode, true, "lobby")
		javaPlayer := filterTestPlayer{
			ctx:         context.Background(),
			gameProfile: profile.GameProfile{ID: unlinked},
		}
		require.True(t, i.AllowClientPluginMessage(floodgate.SkinChannel, javaPlayer, testBackendTarget("lobby")))
	})

	t.Run("nil integration", func(t *testing.T) {
		var i *Integration
		player := bedrockFilterPlayer(t, jconfig.VelocityForwardingMode, data, unlinked)
		require.True(t, i.AllowClientPluginMessage(floodgate.SkinChannel, player, testBackendTarget("lobby")))
	})
}

// A configuration that enables backend Floodgate must register the filter, and
// stopping the integration must clear it again - a stale filter would keep
// dropping Floodgate plugin messages after backendFloodgate was turned off or
// the listener changed.
func TestBackendFloodgateRegistersClientPluginMessageFilter(t *testing.T) {
	cfg := &bedrockconfig.Config{
		FloodgateKeyPath: writeTestFloodgateKey(t),
		BackendFloodgate: bedrockconfig.BackendFloodgate{
			Enabled:        true,
			AllowedServers: []string{"lobby"},
		},
	}
	jp := filterTestProxy(t, jconfig.VelocityForwardingMode)
	integration, err := NewIntegration(context.Background(), jp, cfg)
	require.NoError(t, err)

	require.True(t, clientPluginMessageFilterRegistered(jp),
		"enabling backend Floodgate must install the client plugin message filter")

	integration.Stop()

	require.False(t, clientPluginMessageFilterRegistered(jp),
		"stopping the integration must clear the client plugin message filter")
}

func clientPluginMessageFilterRegistered(p *jproxy.Proxy) bool {
	field := reflect.ValueOf(p).Elem().FieldByName("clientPluginMessageFilter")
	return !field.IsNil()
}

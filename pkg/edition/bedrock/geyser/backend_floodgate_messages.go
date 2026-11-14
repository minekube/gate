package geyser

import (
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	javaconfig "go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

// AllowClientPluginMessage implements proxy.ClientPluginMessageFilter.
//
// A backend Floodgate plugin resolves a session by the Java profile UUID the
// proxy forwarded it (see floodgate.BedrockData.ResolvesProfileUUID). Under
// Velocity forwarding that UUID is the one Gate chose: the linked Java UUID for
// a promoted player, Floodgate's own new UUID(0, xuid) for nobody, and Gate's
// v5 XUID identity for an unlinked player - an identity Floodgate never derives
// and therefore cannot map to a FloodgatePlayer.
//
// Such a session is still registered by the backend Floodgate (Gate re-attaches
// the handshake triplet for allowlisted servers), but every lookup by the
// player's UUID misses, and a Floodgate client plugin message for it takes
// SpigotPluginMessageRegistration's "not a Floodgate player" branch, which kicks
// the player ("Only Floodgate players can send floodgate messages!"). Geyser
// sends exactly such a message - floodgate:skin, carrying the uploaded Bedrock
// skin - about a second after spawn, so an unlinked Bedrock player on a
// backend-Floodgate server is disconnected right after joining.
//
// Gate does not deliver messages a backend would answer that way. The skin is
// not lost: Gate itself applies the Bedrock skin from the GeyserMC skin API
// (Integration.onGameProfile), and a backend Floodgate that subscribed to the
// Geyser skin uploader receives skins over its own websocket.
//
// Linked players keep their Floodgate plugin messages: their forwarded UUID is
// the linked Java UUID, which the backend Floodgate resolves.
func (i *Integration) AllowClientPluginMessage(channel string, player proxy.Player, target proxy.RegisteredServer) bool {
	if i == nil || i.config == nil || !i.config.BackendFloodgate.Enabled {
		return true
	}
	if !floodgate.IsClientChannel(channel) {
		return true
	}
	if !i.backendFloodgateAllowed(target) {
		return true
	}
	geyserConn, ok := FromContext(player.Context())
	if !ok || geyserConn == nil || geyserConn.BedrockData == nil {
		// A Java player that registers a Floodgate channel is not a session this
		// integration speaks for.
		return true
	}
	if i.proxy == nil || i.proxy.Config().Forwarding.Mode != javaconfig.VelocityForwardingMode {
		// Without Velocity forwarding the backend Floodgate sets the player's
		// UUID itself (SpigotDataHandler spoofs FloodgatePlayer.getCorrectUniqueId
		// when there is no proxy data), so it resolves the session whatever
		// identity Gate picked.
		return true
	}
	return geyserConn.BedrockData.ResolvesProfileUUID(player.GameProfile().ID)
}

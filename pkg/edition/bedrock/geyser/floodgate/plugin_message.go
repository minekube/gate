package floodgate

import (
	"strings"

	"go.minekube.com/gate/pkg/util/uuid"
)

// The plugin message channels Floodgate registers on a backend server. Geyser
// (and Floodgate-aware clients) send messages on them to the Floodgate plugin
// running there.
const (
	SkinChannel     = "floodgate:skin"
	FormChannel     = "floodgate:form"
	TransferChannel = "floodgate:transfer"
	PacketChannel   = "floodgate:packet"

	// channelNamespace is the identifier namespace all of the above share. A
	// backend Floodgate plugin treats every message it receives in its namespace
	// as its own, so the namespace is what decides, not the four known names.
	channelNamespace = "floodgate:"
)

// IsClientChannel reports whether channel is a Floodgate plugin message channel,
// i.e. one a backend Floodgate plugin handles itself.
func IsClientChannel(channel string) bool {
	return strings.HasPrefix(channel, channelNamespace)
}

// ResolvesProfileUUID reports whether a Floodgate plugin that received this
// connection's BedrockData can map a player whose Java profile UUID is id back
// to a FloodgatePlayer.
//
// Floodgate resolves a session by the UUID it sees on the backend
// (AbstractPluginMessageHandler/SpigotPluginMessageRegistration look players up
// with that UUID):
//
//   - an unlinked Bedrock player is stored under Floodgate's own XUID UUID,
//     new UUID(0, xuid) - see FloodgateJavaUuid, which is also the registry key
//     (SimpleFloodgateApi.addPlayer keys by FloodgatePlayer.getJavaUniqueId);
//   - a linked player is additionally resolvable by their linked Java UUID
//     (SimpleFloodgateApi.getPlayer falls back to getCorrectUniqueId).
//
// The one identity that does not resolve is Gate's own XUID-derived v5 UUID
// (JavaUuid): Floodgate never derives it, so a plugin only sees it on a session
// it registered under a different key. Callers use this to keep Floodgate
// plugin messages away from a backend that would answer them by kicking the
// player for not being a Floodgate player.
//
// Unknown input is treated as resolvable: this predicate gates dropping a
// message, so it fails open.
func (d *BedrockData) ResolvesProfileUUID(id uuid.UUID) bool {
	if d == nil {
		return true
	}
	// Floodgate's own unlinked identity: a direct hit on its registry key.
	if id == d.FloodgateJavaUuid() {
		return true
	}
	own, err := d.JavaUuid()
	if err != nil {
		return true
	}
	// Any other UUID is either a linked Java identity (Floodgate looks those up
	// by getCorrectUniqueId) or an identity Gate applied from verified
	// provenance; only the XUID v5 identity is one Floodgate cannot know.
	return id != own
}

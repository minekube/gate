package resourcepack

import "go.minekube.com/common/minecraft/component"

// ResourcePackInfo is resource-pack options for Player.SendResourcePack.
type ResourcePackInfo struct {
	ID uuid.UUID // The ID of this resource-pack.
	// The download link the resource-pack can be found at.
	URL string
	// The SHA-1 hash of the provided resource pack.
	//
	// Note: It is recommended to always set this hash.
	// If this hash is not set/not present then the client will always download
	// the resource pack even if it may still be cached. By having this hash present,
	// the client will check first whether a resource pack by this hash is cached
	// before downloading.
	Hash []byte
	// Whether the acceptance of the resource-pack is enforced.
	//
	// Sets the resource-pack as required to play on the network.
	// This feature was introduced in 1.17.
	// Setting this to true has one of two effects:
	// If the client is on 1.17 or newer:
	//  - The resource-pack prompt will display without a decline button
	//  - Accept or disconnect are the only available options but players may still press escape.
	//  - Forces the resource-pack offer prompt to display even if the player has
	//    previously declined or disabled resource packs
	//  - The player will be disconnected from the network if they close/skip the prompt.
	// If the client is on a version older than 1.17:
	//   - If the player accepts the resource pack or has previously accepted a resource-pack
	//     then nothing else will happen.
	//   - If the player declines the resource pack or has previously declined a resource-pack
	//     the player will be disconnected from the network
	ShouldForce bool
	// The optional message that is displayed on the resource-pack prompt.
	// This is only displayed if the client version is 1.17 or newer.
	Prompt component.Component
	Origin ResourcePackOrigin // The origin of the resource-pack.
}

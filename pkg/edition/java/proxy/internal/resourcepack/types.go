package resourcepack

import (
	"encoding/hex"
	"fmt"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
	"regexp"
)

// Info is resource-pack info for a resource-pack.
type Info struct {
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
	Hash Hash
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
	Origin Origin // The origin of the resource-pack.
}

// Origin represents the origin of the resource-pack.
type Origin byte

// Type of resource-pack origin.
const (
	PluginOnProxyOrigin Origin = iota
	DownstreamServerOrigin
)

// InfoForRequest creates a new resource-pack info from a resource-pack request.
func InfoForRequest(r *packet.ResourcePackRequest) (*Info, error) {
	if r.URL == "" {
		return nil, fmt.Errorf("resource pack URL is empty")
	}
	info := &Info{
		ID:          r.ID,
		URL:         r.URL,
		ShouldForce: r.Required,
		Prompt:      r.Prompt.AsComponentOrNil(),
	}
	if r.Hash != "" && Hash(r.Hash).Validate() {
		var err error
		info.Hash, err = hex.DecodeString(r.Hash)
		if err != nil {
			return nil, fmt.Errorf("error decoding resource pack hash: %w", err)
		}
	}
	return info, nil
}

// RequestPacket creates a new resource-pack request from the info.
func (i *Info) RequestPacket(protocol proto.Protocol) *packet.ResourcePackRequest {
	req := &packet.ResourcePackRequest{
		ID:       i.ID,
		URL:      i.URL,
		Required: i.ShouldForce,
		Prompt:   chat.FromComponentProtocol(i.Prompt, protocol),
	}
	if len(i.Hash) != 0 {
		req.Hash = hex.EncodeToString(i.Hash)
	}
	return req
}

// ResponseBundle is a response bundle for a resource-pack.
type ResponseBundle struct {
	// The ID of the resource-pack.
	ID uuid.UUID
	// The hash of the resource-pack.
	Hash Hash
	// The status of the resource-pack.
	Status packet.ResponseStatus
}

// ResponsePacket creates a new resource-pack response from the response bundle.
func (r *ResponseBundle) ResponsePacket() *packet.ResourcePackResponse {
	return &packet.ResourcePackResponse{
		ID:     r.ID,
		Hash:   string(r.Hash),
		Status: r.Status,
	}
}

// BundleForResponse creates a new response bundle from a resource-pack response.
func BundleForResponse(r *packet.ResourcePackResponse) *ResponseBundle {
	return &ResponseBundle{
		ID:     r.ID,
		Hash:   Hash(r.Hash),
		Status: r.Status,
	}
}

// Hash is a SHA-1 hash of a resource-pack.
type Hash []byte

// sha1HexRegex is a regex to match a SHA-1 hash.
var sha1HexRegex = regexp.MustCompile(`[0-9a-fA-F]{40}`)

// Validate returns true if the given hash is a plausible SHA-1 hash used for resource-packs.
func (h Hash) Validate() bool {
	return sha1HexRegex.MatchString(string(h))
}

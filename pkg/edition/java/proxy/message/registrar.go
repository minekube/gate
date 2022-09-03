package message

import (
	"sync"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/sets"
)

// ChannelRegistrar is a plugin message channel registrar.
type ChannelRegistrar struct {
	mu          sync.RWMutex // Protects following fields
	identifiers map[string]ChannelIdentifier
}

// NewChannelRegistrar returns a new ChannelRegistrar.
func NewChannelRegistrar() *ChannelRegistrar {
	return &ChannelRegistrar{identifiers: map[string]ChannelIdentifier{}}
}

// Register registers the specified message identifiers to listen on so you can
// intercept plugin messages on the channel using the PluginMessageEvent.
func (r *ChannelRegistrar) Register(ids ...ChannelIdentifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		r.identifiers[id.ID()] = id
		if legacy, ok := id.(*LegacyChannelIdentifier); ok {
			rewritten := plugin.TransformLegacyToModernChannel(legacy.ID())
			r.identifiers[rewritten] = id
		}
	}
}

// Unregister removes the intent to listen for the specified channels.
func (r *ChannelRegistrar) Unregister(ids ...ChannelIdentifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		delete(r.identifiers, id.ID())
		if legacy, ok := id.(*LegacyChannelIdentifier); ok {
			rewritten := plugin.TransformLegacyToModernChannel(legacy.ID())
			delete(r.identifiers, rewritten)
		}
	}
}

// ChannelsForProtocol returns all the channel names
// to register depending on the Minecraft protocol version.
func (r *ChannelRegistrar) ChannelsForProtocol(protocol proto.Protocol) sets.String {
	if protocol.GreaterEqual(version.Minecraft_1_13) {
		return r.ModernChannelIDs()
	}
	return r.LegacyChannelIDs()
}

// ModernChannelIDs returns all channel IDs (as strings)
// for use with Minecraft 1.13 and above.
func (r *ChannelRegistrar) ModernChannelIDs() sets.String {
	r.mu.RLock()
	ids := r.identifiers
	r.mu.RUnlock()
	ss := sets.String{}
	for _, i := range ids {
		if _, ok := i.(*MinecraftChannelIdentifier); ok {
			ss.Insert(i.ID())
		} else {
			ss.Insert(plugin.TransformLegacyToModernChannel(i.ID()))
		}
	}
	return ss
}

// LegacyChannelIDs returns all legacy channel IDs.
func (r *ChannelRegistrar) LegacyChannelIDs() sets.String {
	r.mu.RLock()
	ids := r.identifiers
	r.mu.RUnlock()
	ss := sets.String{}
	for _, i := range ids {
		ss.Insert(i.ID())
	}
	return ss
}

// FromID returns the registered channel identifier for the specified ID.
func (r *ChannelRegistrar) FromID(channel string) (ChannelIdentifier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.identifiers[channel]
	return id, ok
}

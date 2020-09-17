package message

import "go.minekube.com/common/minecraft/key"

// MinecraftChannelIdentifier is a Minecraft 1.13+ channel identifier.
type MinecraftChannelIdentifier struct {
	key.Key
}

func (m *MinecraftChannelIdentifier) ID() string {
	return m.String()
}

var (
	_ ChannelIdentifier = (*MinecraftChannelIdentifier)(nil)
	_ ChannelIdentifier = (*LegacyChannelIdentifier)(nil)
)

// LegacyChannelIdentifier is a legacy channel identifier (for Minecraft 1.12 and below).
// For modern 1.13 plugin messages, please see MinecraftChannelIdentifier.
type LegacyChannelIdentifier string // Has just a name field, no namespace and value.

func (l LegacyChannelIdentifier) ID() string {
	return string(l)
}

package proxy

import (
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/util/profile"
)

// connectionType is a client connection type.
type connectionType interface {
	initialClientPhase() clientConnectionPhase
	initialBackendPhase() backendConnectionPhase
	addGameProfileTokensIfRequired(original *profile.GameProfile, forwardingType config.ForwardingMode) *profile.GameProfile
}

type connType struct {
	initialClientPhase_  clientConnectionPhase
	initialBackendPhase_ backendConnectionPhase
}

var _ connectionType = (*connType)(nil)

func (c *connType) initialClientPhase() clientConnectionPhase {
	return c.initialClientPhase_
}

func (c *connType) initialBackendPhase() backendConnectionPhase {
	return c.initialBackendPhase_
}
func (*connType) addGameProfileTokensIfRequired(original *profile.GameProfile, _ config.ForwardingMode) *profile.GameProfile {
	return original
}

type legacyForgeConnType struct {
	*connType
}

func (*legacyForgeConnType) addGameProfileTokensIfRequired(original *profile.GameProfile, forwardingType config.ForwardingMode) *profile.GameProfile {
	// We can't forward the FML token to the server when we are running in legacy forwarding mode,
	// since both use the "hostname" field in the handshake. We add a special property to the
	// profile instead, which will be ignored by non-Forge servers and can be intercepted by a
	// Forge coremod, such as SpongeForge.
	if forwardingType == config.LegacyForwardingMode {
		original.Properties = append(original.Properties, profile.Property{Name: "forgeClient", Value: "true"})
	}
	return original
}

var (
	// Indicates that the connection has yet to reach the
	// point where we have a definitive answer as to what
	// type of connection we have.
	undeterminedConnectionType connectionType = &connType{
		initialClientPhase_:  vanillaClientPhase,
		initialBackendPhase_: unknownBackendPhase,
	}
	// Indicates that a connection is a Vanilla connection.
	vanillaConnectionType connectionType = &connType{
		initialClientPhase_:  vanillaClientPhase,
		initialBackendPhase_: vanillaBackendPhase,
	}
	// 1.7 version
	undetermined17ConnectionType = &connType{
		initialClientPhase_:  notStartedLegacyForgeHandshakeClientPhase,
		initialBackendPhase_: unknownBackendPhase,
	}
	// Indicates that the connection is a 1.8-1.12 Forge connection.
	LegacyForge connectionType = &legacyForgeConnType{
		connType: &connType{
			initialClientPhase_:  notStartedLegacyForgeHandshakeClientPhase,
			initialBackendPhase_: notStartedLegacyForgeHandshakeBackendPhase,
		},
	}
)

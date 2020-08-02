package proxy

import (
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/util/gameprofile"
)

// connectionType is a client connection type.
type connectionType interface {
	initialClientPhase() clientConnectionPhase
	initialBackendPhase() backendConnectionPhase
	addGameProfileTokensIfRequired(original *gameprofile.GameProfile, forwardingType config.ForwardingMode) *gameprofile.GameProfile
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
func (*connType) addGameProfileTokensIfRequired(original *gameprofile.GameProfile, _ config.ForwardingMode) *gameprofile.GameProfile {
	return original
}

type legacyForgeConnType struct {
	*connType
}

var isForgeClientProperty = gameprofile.NewProperty("forgeClient", "true", "")

func (*legacyForgeConnType) addGameProfileTokensIfRequired(original *gameprofile.GameProfile, forwardingType config.ForwardingMode) *gameprofile.GameProfile {
	// We can't forward the FML token to the server when we are running in legacy forwarding mode,
	// since both use the "hostname" field in the handshake. We add a special property to the
	// profile instead, which will be ignored by non-Forge servers and can be intercepted by a
	// Forge coremod, such as SpongeForge.
	if forwardingType == config.LegacyForwardingMode {
		original.Properties = append(original.Properties, isForgeClientProperty)
		return original // TODO make game profile an interface
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

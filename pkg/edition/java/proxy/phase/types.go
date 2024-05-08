package phase

import (
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/profile"
)

// The connection types supported.
var (
	// Undetermined indicates that the connection has yet to reach the
	// point where we have a definitive answer as to what type of connection we have.
	Undetermined ConnectionType = &connType{
		initialClientPhase:  VanillaClientPhase,
		initialBackendPhase: UnknownBackendPhase,
	}
	// Vanilla indicates that a connection is a vanilla connection.
	Vanilla ConnectionType = &connType{
		initialClientPhase:  VanillaClientPhase,
		initialBackendPhase: VanillaBackendPhase,
	}
	// Undetermined17 is a 1.7 version connection type.
	Undetermined17 ConnectionType = &connType{
		initialClientPhase:  NotStartedLegacyForgeHandshakeClientPhase,
		initialBackendPhase: UnknownBackendPhase,
	}
	// LegacyForge indicates that the connection is a 1.8-1.12 Forge connection.
	LegacyForge ConnectionType = &legacyForgeConnType{
		connType: &connType{
			initialClientPhase:  NotStartedLegacyForgeHandshakeClientPhase,
			initialBackendPhase: NotStartedLegacyForgeHandshakeBackendPhase,
		},
	}
	ModernForge ConnectionType = &connType{
		initialClientPhase:  VanillaClientPhase,
		initialBackendPhase: VanillaBackendPhase,
	}
)

// ConnectionType is a connection type.
type ConnectionType interface {
	InitialClientPhase() ClientConnectionPhase
	InitialBackendPhase() BackendConnectionPhase
	AddGameProfileTokensIfRequired(
		original *profile.GameProfile,
		forwardingType config.ForwardingMode,
	) *profile.GameProfile
}

type connType struct {
	initialClientPhase  ClientConnectionPhase
	initialBackendPhase BackendConnectionPhase
}

var _ ConnectionType = (*connType)(nil)

func (c *connType) InitialClientPhase() ClientConnectionPhase {
	return c.initialClientPhase
}

func (c *connType) InitialBackendPhase() BackendConnectionPhase {
	return c.initialBackendPhase
}
func (*connType) AddGameProfileTokensIfRequired(
	original *profile.GameProfile,
	_ config.ForwardingMode,
) *profile.GameProfile {
	return original
}

type legacyForgeConnType struct{ *connType }

func (*legacyForgeConnType) AddGameProfileTokensIfRequired(
	original *profile.GameProfile,
	forwardingType config.ForwardingMode,
) *profile.GameProfile {
	// We can't forward the FML token to the server when we are running in legacy forwarding mode,
	// since both use the "hostname" field in the handshake. We add a special property to the
	// profile instead, which will be ignored by non-Forge servers and can be intercepted by a
	// Forge coremod, such as SpongeForge.
	if forwardingType == config.LegacyForwardingMode || forwardingType == config.BungeeGuardFowardingMode {
		original.Properties = append(original.Properties, profile.Property{Name: "forgeClient", Value: "true"})
	}
	return original
}

package phase

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/gate/proto"
)

var (
	// VanillaBackendPhase is a vanilla backend connection.
	VanillaBackendPhase BackendConnectionPhase = &unimplementedBackendPhase{}
	// UnknownBackendPhase indicated the backend connection is unknown at this time.
	UnknownBackendPhase BackendConnectionPhase = &unknownBackendPhase{}
	// InTransitionBackendPhase is a special backend phase used to indicate that this connection is about to become
	// obsolete (transfer to a new server, for instance) and that Forge messages ought to be
	// forwarded on to an in-flight connection instead.
	InTransitionBackendPhase BackendConnectionPhase = &inTransitionBackendPhase{}
)

// BackendConnectionPhase allows for simple tracking of the
// phase that the Legacy Forge handshake is in (server side).
type BackendConnectionPhase interface {
	// Handle a plugin message in the context of this phase.
	Handle(
		player PacketWriter,
		backend BackendConnectionPhaseSetter,
		server ConnectionTypeSetter,
		resetter LegacyForgeHandshakeResetter,
		msg *plugin.Message,
	) bool
	// ConsideredComplete indicates whether the connection is considered complete.
	ConsideredComplete() bool
	// OnDepartForNewServer fired when the provided server connection is about to be terminated
	// because the provided player is connecting to a new server.
	OnDepartForNewServer(
		player PacketWriter,
		phase ClientConnectionPhase,
		setter ClientConnectionPhaseSetter,
	)
}

type (
	BackendConnectionPhaseSetter interface {
		SetPhase(BackendConnectionPhase)
	}
	ClientConnectionPhaseSetter interface {
		SetPhase(ClientConnectionPhase)
	}
	ConnectionTypeSetter interface {
		SetType(ConnectionType)
	}
	PacketWriter interface {
		WritePacket(proto.Packet) error
	}
	LegacyForgeHandshakeResetter interface {
		SendLegacyForgeHandshakeResetPacket()
	}
)

type (
	unknownBackendPhase      struct{ unimplementedBackendPhase }
	inTransitionBackendPhase struct{ unimplementedBackendPhase }
)

func (unknownBackendPhase) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	// The connection may be legacy forge. If so, the Forge handler will deal with this
	// for us. Otherwise, we have nothing to do.
	return NotStartedLegacyForgeHandshakeBackendPhase.Handle(player, backend, server, resetter, msg)
}

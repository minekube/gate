package phase

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
)

var (
	VanillaClientPhase ClientConnectionPhase = &unimplementedClient{}
)

// ClientConnectionPhase allows for simple tracking of
// the phase that the Legacy Forge handshake is in.
type ClientConnectionPhase interface {
	// Handle a plugin message in the context of this phase.
	// Returns true if handled, false otherwise.
	Handle(
		mi ModInfo,
		client ClientConnectionPhaseSetter,
		player KeepAlive,
		backendConn BackendConn,
		msg *plugin.Message,
	) bool
	// OnFirstJoin performs actions just as the player joins the server.
	OnFirstJoin(setter ClientConnectionPhaseSetter)
	// ConsideredComplete indicates whether the connection is considered complete.
	ConsideredComplete() bool
	Resetter
}

// Resetter can reset the connection phase.
type Resetter interface {
	// ResetConnectionPhase instructs the proxy to reset the connection phase
	// back to its default for the connection type.
	ResetConnectionPhase(PacketWriter, ClientConnectionPhaseSetter)
}

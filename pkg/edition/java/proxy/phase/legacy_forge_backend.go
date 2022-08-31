package phase

import (
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
)

var (
	// NotStartedLegacyForgeHandshakeBackendPhase indicates that the handshake has not started, used for UnknownBackendPhase.
	NotStartedLegacyForgeHandshakeBackendPhase BackendConnectionPhase = &notStarted{}
	// HelloLegacyForgeHandshakeBackendPhase sent a hello to the client, waiting for a hello back before sending the mod list.
	HelloLegacyForgeHandshakeBackendPhase backendConnectionPhase = &hello{}
	// SentModListLegacyForgeHandshakeBackendPhase is the mod list from the client has been accepted and a server mod list
	// has been sent. Waiting for the client to acknowledge.
	SentModListLegacyForgeHandshakeBackendPhase backendConnectionPhase = &sentModList{}
	// SentServerDataLegacyForgeHandshakeBackendPhase is the server data is being sent or has been sent, and is waiting for
	// the client to acknowledge it has processed this.
	SentServerDataLegacyForgeHandshakeBackendPhase backendConnectionPhase = &sentServerData{}
	// WaitingAckLegacyForgeHandshakeBackendPhase is waiting for the client to acknowledge before completing handshake.
	WaitingAckLegacyForgeHandshakeBackendPhase backendConnectionPhase = &waitingAck{}
	// CompleteLegacyForgeHandshakeBackendPhase is the server has completed the handshake and will continue after the client ACK.
	CompleteLegacyForgeHandshakeBackendPhase backendConnectionPhase = &complete{}
)

type (
	notStarted     struct{ unimplementedBackendPhase }
	hello          struct{ unimplementedBackendPhase }
	sentModList    struct{ unimplementedBackendPhase }
	sentServerData struct{ unimplementedBackendPhase }
	waitingAck     struct{ unimplementedBackendPhase }
	complete       struct{ unimplementedBackendPhase }

	unimplementedBackendPhase struct{}
)

func (notStarted) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeBackendMessage(player, backend, server, resetter, msg,
		intPtr(forge.ServerHelloDiscriminator),
		HelloLegacyForgeHandshakeBackendPhase,
	)
}
func (hello) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeBackendMessage(player, backend, server, resetter, msg,
		intPtr(forge.ModListDiscriminator),
		SentModListLegacyForgeHandshakeBackendPhase,
	)
}
func (hello) onTransitionToNewPhase(
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
) {
	// We must always reset the handshake before a modded connection is established if
	// we haven't done so already.
	server.SetType(LegacyForge)
	resetter.SendLegacyForgeHandshakeResetPacket()
}
func (sentModList) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeBackendMessage(player, backend, server, resetter, msg,
		intPtr(forge.RegistryDiscriminator),
		SentServerDataLegacyForgeHandshakeBackendPhase,
	)
}
func (sentServerData) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeBackendMessage(player, backend, server, resetter, msg,
		intPtr(forge.AckDiscriminator),
		WaitingAckLegacyForgeHandshakeBackendPhase,
	)
}
func (waitingAck) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeBackendMessage(player, backend, server, resetter, msg,
		intPtr(forge.AckDiscriminator),
		CompleteLegacyForgeHandshakeBackendPhase,
	)
}

type backendConnectionPhase interface {
	BackendConnectionPhase
	onTransitionToNewPhase(
		server ConnectionTypeSetter,
		resetter LegacyForgeHandshakeResetter,
	)
}

func handleLegacyForgeBackendMessage(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
	packetToAdvanceOn *int,
	nextPhase backendConnectionPhase,
) bool {
	if msg.Channel != forge.LegacyHandshakeChannel {
		// Not handled, fallback
		return false
	}

	if packetToAdvanceOn != nil {
		discriminator, ok := forge.HandshakePacketDiscriminator(msg)
		if ok && discriminator == byte(*packetToAdvanceOn) {

			phaseToTransitionTo := nextPhase
			phaseToTransitionTo.onTransitionToNewPhase(server, resetter)

			// Update phase on server
			backend.SetPhase(nextPhase)
		}
	}

	// Write the packet to the player, we don't need it now.
	_ = player.WritePacket(msg)
	return true
}

func intPtr(i int) *int { return &i }

func (unimplementedBackendPhase) Handle(
	player PacketWriter,
	backend BackendConnectionPhaseSetter,
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
	msg *plugin.Message,
) bool {
	return false
}
func (unimplementedBackendPhase) ConsideredComplete() bool { return false }
func (unimplementedBackendPhase) OnDepartForNewServer(
	player PacketWriter,
	phase ClientConnectionPhase,
	setter ClientConnectionPhaseSetter,
) {
	// If the server we are departing is modded, we must always reset the client's handshake.
	phase.ResetConnectionPhase(player, setter)
}
func (unimplementedBackendPhase) onTransitionToNewPhase(
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
) {
}

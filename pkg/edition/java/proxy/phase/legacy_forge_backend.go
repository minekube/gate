package phase

import (
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
)

var (
	// NotStartedLegacyForgeHandshakeBackendPhase indicates that the handshake has not started, used for UnknownBackendPhase.
	NotStartedLegacyForgeHandshakeBackendPhase BackendConnectionPhase = &notStartedBackend{}
	// HelloLegacyForgeHandshakeBackendPhase sent a hello to the client, waiting for a hello back before sending the mod list.
	HelloLegacyForgeHandshakeBackendPhase backendConnectionPhase = &helloBackend{}
	// SentModListLegacyForgeHandshakeBackendPhase is the mod list from the client has been accepted and a server mod list
	// has been sent. Waiting for the client to acknowledge.
	SentModListLegacyForgeHandshakeBackendPhase backendConnectionPhase = &sentModListBackend{}
	// SentServerDataLegacyForgeHandshakeBackendPhase is the server data is being sent or has been sent, and is waiting for
	// the client to acknowledge it has processed this.
	SentServerDataLegacyForgeHandshakeBackendPhase backendConnectionPhase = &sentServerDataBackend{}
	// WaitingAckLegacyForgeHandshakeBackendPhase is waiting for the client to acknowledge before completing handshake.
	WaitingAckLegacyForgeHandshakeBackendPhase backendConnectionPhase = &waitingAckBackend{}
	// CompleteLegacyForgeHandshakeBackendPhase is the server has completed the handshake and will continue after the client ACK.
	CompleteLegacyForgeHandshakeBackendPhase backendConnectionPhase = &completeBackend{}
)

type (
	notStartedBackend     struct{ unimplementedBackendPhase }
	helloBackend          struct{ unimplementedBackendPhase }
	sentModListBackend    struct{ unimplementedBackendPhase }
	sentServerDataBackend struct{ unimplementedBackendPhase }
	waitingAckBackend     struct{ unimplementedBackendPhase }
	completeBackend       struct{ unimplementedBackendPhase }
)

func (notStartedBackend) Handle(
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
func (helloBackend) Handle(
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
func (helloBackend) onTransitionToNewPhase(
	server ConnectionTypeSetter,
	resetter LegacyForgeHandshakeResetter,
) {
	// We must always reset the handshake before a modded connection is established if
	// we haven't done so already.
	server.SetType(LegacyForge)
	resetter.SendLegacyForgeHandshakeResetPacket()
}
func (sentModListBackend) Handle(
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
func (sentServerDataBackend) Handle(
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
func (waitingAckBackend) Handle(
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

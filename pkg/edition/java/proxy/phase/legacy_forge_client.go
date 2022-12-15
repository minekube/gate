package phase

import (
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/forge/modinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
)

var (
	// NotStartedLegacyForgeHandshakeClientPhase no handshake packets have yet been sent.
	// Transition HelloLegacyForgeHandshakeClientPhase when the ClientHello is sent.
	NotStartedLegacyForgeHandshakeClientPhase ClientConnectionPhase = &notStartedClient{}
	// HelloLegacyForgeHandshakeClientPhase Client and Server exchange pleasantries.
	// Transition to ModListLegacyForgeHandshakeClientPhase when the ModList is sent.
	HelloLegacyForgeHandshakeClientPhase clientConnectionPhase = &helloClient{}
	// ModListLegacyForgeHandshakeClientPhase the Mod list is sent to the server, captured by the proxy.
	// Transition to WaitingServerDataLegacyForgeHandshakeClientPhase when an ACK is sent, which
	// indicates to the server to start sending state data.
	ModListLegacyForgeHandshakeClientPhase clientConnectionPhase = &modListClient{}
	// WaitingServerDataLegacyForgeHandshakeClientPhase Waiting for state data to be received.
	// Transition to WaitingServerCompleteLegacyForgeHandshakeClientPhase when this is complete
	// and the client sends an ACK packet to confirm
	WaitingServerDataLegacyForgeHandshakeClientPhase clientConnectionPhase = &waitingServerDataClient{}
	// WaitingServerCompleteLegacyForgeHandshakeClientPhase Waiting on the server to send another ACK.
	// Transition to PendingCompleteLegacyForgeHandshakeClientPhase when client sends another ACK.
	WaitingServerCompleteLegacyForgeHandshakeClientPhase clientConnectionPhase = &waitingServerCompleteClient{}
	// PendingCompleteLegacyForgeHandshakeClientPhase Waiting on the server to send yet another ACK.
	// Transition to {@link #COMPLETE} when client sends another ACK
	PendingCompleteLegacyForgeHandshakeClientPhase clientConnectionPhase = &pendingCompleteClient{}
	// CompleteLegacyForgeHandshakeClientPhase the handshake is complete.
	// The handshake can be reset.
	//
	// Note that a successful connection to a server does not mean that
	// we will be in this state. After a handshake reset, if the next server
	// is vanilla we will still be in the NOT_STARTED phase,
	// which means we must NOT send a reset packet. This is handled by
	// overriding the resetConnectionPhase(*connectedPlayer) in this
	// element (it is usually a no-op).
	CompleteLegacyForgeHandshakeClientPhase clientConnectionPhase = &completeClient{}
)

type (
	notStartedClient            struct{ unimplementedClient }
	helloClient                 struct{ unimplementedClient }
	modListClient               struct{ unimplementedClient }
	waitingServerDataClient     struct{ unimplementedClient }
	waitingServerCompleteClient struct{ unimplementedClient }
	pendingCompleteClient       struct{ unimplementedClient }
	completeClient              struct{ unimplementedClient }

	unimplementedClient struct{}
)

func (unimplementedClient) ConsideredComplete() bool         { return true }
func (notStartedClient) ConsideredComplete() bool            { return false }
func (helloClient) ConsideredComplete() bool                 { return false }
func (modListClient) ConsideredComplete() bool               { return false }
func (waitingServerDataClient) ConsideredComplete() bool     { return false }
func (waitingServerCompleteClient) ConsideredComplete() bool { return false }
func (pendingCompleteClient) ConsideredComplete() bool       { return false }
func (completeClient) ConsideredComplete() bool              { return true }

func (notStartedClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		intPtr(forge.ClientHelloDiscriminator),
		HelloLegacyForgeHandshakeClientPhase,
	)
}
func (helloClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		intPtr(forge.ModListDiscriminator),
		ModListLegacyForgeHandshakeClientPhase,
	)
}
func (modListClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		intPtr(forge.AckDiscriminator),
		WaitingServerDataLegacyForgeHandshakeClientPhase,
	)
}
func (waitingServerDataClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		intPtr(forge.AckDiscriminator),
		WaitingServerCompleteLegacyForgeHandshakeClientPhase,
	)
}
func (waitingServerCompleteClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		intPtr(forge.AckDiscriminator),
		PendingCompleteLegacyForgeHandshakeClientPhase,
	)
}
func (pendingCompleteClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		intPtr(forge.AckDiscriminator),
		CompleteLegacyForgeHandshakeClientPhase,
	)
}
func (completeClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return handleLegacyForgeClientMessage(mi, player, client, backendConn, msg,
		nil,
		CompleteLegacyForgeHandshakeClientPhase, // no next
	)
}

func (notStartedClient) OnFirstJoin(setter ClientConnectionPhaseSetter) {
	// We have something special to do for legacy Forge servers - during first connection the FML
	// handshake will getNewPhase to complete regardless. Thus, we need to ensure that a reset
	// packet is ALWAYS sent on first switch.
	//
	// As we know that calling this branch only happens on first join, we set that if we are a
	// Forge client that we must reset on the next switch.
	setter.SetPhase(CompleteLegacyForgeHandshakeClientPhase)
}

func (notStartedClient) onHandle(
	mi ModInfo,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	// If we stay in this phase, we do nothing because it means the packet wasn't handled.
	// Returning false indicates this
	return false
}
func (c modListClient) onHandle(
	mi ModInfo,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	// Read the mod list if we haven't already.
	if mi.ModInfo() == nil {
		mods, err := forge.ReadMods(msg)
		if err == nil {
			if len(mods) != 0 {
				mi.SetModInfo(&modinfo.ModInfo{
					Type: "FML",
					Mods: mods,
				})
			}
		}
	}

	return c.unimplementedClient.onHandle(mi, player, backendConn, msg)
}
func (c completeClient) onHandle(
	mi ModInfo,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	c.unimplementedClient.onHandle(mi, player, backendConn, msg)

	// just in case the timing is awful
	if player.SendKeepAlive() != nil {
		return false
	}

	backendConn.FlushQueuedPluginMessages()

	return true
}
func (c completeClient) ResetConnectionPhase(
	player PacketWriter,
	setter ClientConnectionPhaseSetter,
) {
	_ = player.WritePacket(forge.ResetPacket())
	setter.SetPhase(NotStartedLegacyForgeHandshakeClientPhase)
}

type (
	ModInfo interface {
		ModInfo() *modinfo.ModInfo
		SetModInfo(*modinfo.ModInfo)
	}
	KeepAlive interface {
		SendKeepAlive() error
	}
	BackendConn interface {
		PacketWriter
		FlushQueuedPluginMessages()
	}
	clientConnectionPhase interface {
		ClientConnectionPhase
		onHandle(
			mi ModInfo,
			player KeepAlive,
			backendConn BackendConn,
			msg *plugin.Message,
		) bool
	}
)

func handleLegacyForgeClientMessage(
	mi ModInfo,
	player KeepAlive,
	client ClientConnectionPhaseSetter,
	backendConn BackendConn,
	msg *plugin.Message,
	packetToAdvanceOn *int,
	nextPhase clientConnectionPhase,
) bool {
	if backendConn != nil && msg.Channel == forge.LegacyHandshakeChannel {
		// Get the phase and check if we need to start the next phase.
		if packetToAdvanceOn != nil {
			discriminator, ok := forge.HandshakePacketDiscriminator(msg)
			if ok && discriminator == byte(*packetToAdvanceOn) {
				// Update phase on player
				client.SetPhase(nextPhase)

				// Perform phase handling
				return nextPhase.onHandle(mi, player, backendConn, msg)
			}
		}
	}

	// Not handled, fallback
	return false
}

// Handle a plugin message in the context of this phase.
// Returns true if handled, false otherwise.
func (unimplementedClient) Handle(
	mi ModInfo,
	client ClientConnectionPhaseSetter,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	return false
}
func (unimplementedClient) ResetConnectionPhase(PacketWriter, ClientConnectionPhaseSetter) {}
func (unimplementedClient) OnFirstJoin(setter ClientConnectionPhaseSetter)                 {}
func (unimplementedClient) onHandle(
	mi ModInfo,
	player KeepAlive,
	backendConn BackendConn,
	msg *plugin.Message,
) bool {
	_ = backendConn.WritePacket(msg)

	// We handled the packet. No need to continue processing.
	return true
}

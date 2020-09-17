package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/util/modinfo"
	"strings"
)

// clientConnectionPhase provides connection phase specific actions.
type clientConnectionPhase interface {
	// Handle a plugin message in the context of this phase.
	// Returns true if handled, false otherwise.
	handle(*serverConnection, *plugin.Message) bool
	// Instruct the proxy to reset the connection phase
	// back to its default for the connection type.
	resetConnectionPhase(*connectedPlayer)
	// Perform actions just as the player joins the server.
	onFirstJoin(*connectedPlayer)
	// Indicates whether the connection is considered complete.
	consideredComplete() bool
}

var vanillaClientPhase clientConnectionPhase = &noOpClientPhase{}

type noOpClientPhase struct{}

func (noOpClientPhase) handle(*serverConnection, *plugin.Message) bool { return false }
func (noOpClientPhase) resetConnectionPhase(*connectedPlayer)          {}
func (noOpClientPhase) onFirstJoin(*connectedPlayer)                   {}
func (noOpClientPhase) consideredComplete() bool                       { return true }

type legacyForgeHandshakeClientPhase struct {
	packetToAdvanceOn     *int                                                                           // nil-able
	nextPhase_            *legacyForgeHandshakeClientPhase                                               // nil-able
	onHandle_             func(p *connectedPlayer, msg *plugin.Message, backendConn *minecraftConn) bool // nil-able
	resetConnectionPhase_ func(p *connectedPlayer)                                                       // nil-able
	onFirstJoin_          func(p *connectedPlayer)                                                       // nil-able
	consideredComplete_   bool
}

var (
	// No handshake packets have yet been sent. Transition to hello when the ClientHello is sent.
	notStartedLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		packetToAdvanceOn: intPtr(forge.ClientHelloDiscriminator),
		nextPhase_:        helloLegacyForgeHandshakeClientPhase,
		onFirstJoin_: func(p *connectedPlayer) {
			// We have something special to do for legacy Forge servers - during first connection the FML
			// handshake will newPhase() to complete regardless. Thus, we need to ensure that a reset
			// packet is ALWAYS sent on first switch.
			//
			// As we know that calling this branch only happens on first join, we set that if we are a
			// Forge client that we must reset on the next switch.
			p.setPhase(completeLegacyForgeHandshakeClientPhase)
		},
		onHandle_: func(p *connectedPlayer, msg *plugin.Message, backendConn *minecraftConn) bool {
			// If we stay in this phase, we do nothing because it means the packet wasn't handled.
			// Returning false indicates this.
			return false
		},
	}
	// Client and Server exchange pleasantries. Transition to modList when the ModList is sent.
	helloLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		packetToAdvanceOn: intPtr(forge.ModListDiscriminator),
		nextPhase_:        modListLegacyForgeHandshakeClientPhase,
	}
	// The Mod list is sent to the server, captured by Velocity.
	// Transition to waitingServerDataLegacyForgeHandshakeClientPhase when an ACK is sent, which
	// indicates to the server to start sending state data.
	modListLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		packetToAdvanceOn: intPtr(forge.AckDiscriminator),
		nextPhase_:        waitingServerDataLegacyForgeHandshakeClientPhase,
		onHandle_:         nil, // see init()
	}
	// Waiting for state data to be received.
	// Transition to waitingServer
	// when this is complete and the client sends an ACK packet to confirm.
	waitingServerDataLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		packetToAdvanceOn: intPtr(forge.AckDiscriminator),
		nextPhase_:        waitingServerCompleteLegacyForgeHandshakeClientPhase,
	}
	// Waiting on the server to send another ACK.
	// Transition to pending when client sends another ACK.
	waitingServerCompleteLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		packetToAdvanceOn: intPtr(forge.AckDiscriminator),
		nextPhase_:        pendingLegacyForgeHandshakeClientPhase,
	}
	// Waiting on the server to send yet another ACK.
	// Transition to complete when client sends another ACK.
	pendingLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		packetToAdvanceOn: intPtr(forge.AckDiscriminator),
		nextPhase_:        completeLegacyForgeHandshakeClientPhase,
	}
	// The handshake is complete. The handshake can be reset.
	//
	// Note that a successful connection to a server does not mean that
	// we will be in this state. After a handshake reset, if the next server
	// is vanilla we will still be in the NOT_STARTED phase,
	// which means we must NOT send a reset packet. This is handled by
	// overriding the resetConnectionPhase(*connectedPlayer) in this
	// element (it is usually a no-op).
	completeLegacyForgeHandshakeClientPhase = &legacyForgeHandshakeClientPhase{
		consideredComplete_:   true,
		onHandle_:             nil, // see init()
		resetConnectionPhase_: nil, // see init()
	}
)

func init() {
	modListLegacyForgeHandshakeClientPhase.onHandle_ = func(p *connectedPlayer, msg *plugin.Message, backendConn *minecraftConn) bool {
		// Read the mod list if we haven't already
		if p.ModInfo() == nil {
			mods, err := forge.ReadMods(msg)
			if err != nil {
				return false
			}
			if len(mods) != 0 {
				p.setModInfo(&modinfo.ModInfo{
					Type: "FML",
					Mods: mods,
				})
			}
		}
		return modListLegacyForgeHandshakeClientPhase.onHandle(false, p, msg, backendConn)
	}
	completeLegacyForgeHandshakeClientPhase.onHandle_ = func(p *connectedPlayer, msg *plugin.Message, backendConn *minecraftConn) bool {
		completeLegacyForgeHandshakeClientPhase.onHandle(false, p, msg, backendConn)

		// just in case the timing is awful
		if p.SendKeepAlive() != nil {
			return false
		}

		if play, ok := backendConn.SessionHandler().(*clientPlaySessionHandler); ok {
			play.flushQueuedMessages()
		}

		return true
	}
	completeLegacyForgeHandshakeClientPhase.resetConnectionPhase_ = func(p *connectedPlayer) {
		_ = p.WritePacket(forge.ResetPacket())
		p.setPhase(notStartedLegacyForgeHandshakeClientPhase)
	}
}

func (l *legacyForgeHandshakeClientPhase) handle(sc *serverConnection, message *plugin.Message) bool {
	if sc == nil {
		return false
	}
	backendConn := sc.conn()
	if backendConn != nil && strings.EqualFold(message.Channel, forge.LegacyHandshakeChannel) {
		// Get the phase and check if we need to start the next phase.
		newPhase := l.newPhase(message)
		// Update phase on player
		sc.player.setPhase(newPhase)
		// Perform phase handling
		return newPhase.onHandle(true, sc.player, message, backendConn)
	}
	// Not handled, fallback
	return false
}

func (l *legacyForgeHandshakeClientPhase) onHandle(checkOverridden bool, p *connectedPlayer, msg *plugin.Message, backendConn *minecraftConn) bool {
	if checkOverridden && l.onHandle_ != nil {
		return l.onHandle_(p, msg, backendConn)
	}
	// Send the packet on the server.
	return backendConn.WritePacket(msg) == nil // If true: We handled the packet. No need to continue processing.
}

func (l *legacyForgeHandshakeClientPhase) resetConnectionPhase(p *connectedPlayer) {
	if l.resetConnectionPhase_ != nil {
		l.resetConnectionPhase_(p)
	}
}

func (l *legacyForgeHandshakeClientPhase) onFirstJoin(p *connectedPlayer) {
	if l.onFirstJoin_ != nil {
		l.onFirstJoin_(p)
		return
	}
}

func (l *legacyForgeHandshakeClientPhase) consideredComplete() bool {
	return l.consideredComplete_
}

// Get the phase to act on, depending on the packet that has been sent.
func (l *legacyForgeHandshakeClientPhase) newPhase(message *plugin.Message) *legacyForgeHandshakeClientPhase {
	if l.packetToAdvanceOn != nil {
		discriminator, ok := forge.HandshakePacketDiscriminator(message)
		if ok && discriminator == byte(*l.packetToAdvanceOn) {
			if l.nextPhase_ != nil {
				return l.nextPhase_
			}
		}
	}
	return l
}

var _ clientConnectionPhase = (*legacyForgeHandshakeClientPhase)(nil)

//
//
//
//
//

// backendConnectionPhase provides connection phase specific actions.
type backendConnectionPhase interface {
	// Handle a plugin message in the context of this phase.
	handle(*serverConnection, *plugin.Message) bool
	// Indicates whether the connection is considered complete.
	consideredComplete() bool
	// Fired when the provided server connection is about to be terminated
	// because the provided player is connecting to a new server.
	onDepartForNewServer(*serverConnection)
}

// The backend connection is vanilla.
var vanillaBackendPhase backendConnectionPhase = &noOpBackendPhase{}

type noOpBackendPhase struct{}

func (noOpBackendPhase) handle(*serverConnection, *plugin.Message) bool {
	return false
}
func (noOpBackendPhase) consideredComplete() bool {
	return true
}
func (noOpBackendPhase) onDepartForNewServer(*serverConnection) {}

// The backend connection is unknown at this time.
var unknownBackendPhase backendConnectionPhase = &unknownBackendPhase_{}

type unknownBackendPhase_ struct {
	noOpBackendPhase
}

func (unknownBackendPhase_) consideredComplete() bool {
	return false
}
func (unknownBackendPhase_) handle(sc *serverConnection, msg *plugin.Message) bool {
	// The connection may be legacy forge. If so, the Forge handler will deal with this
	// for us. Otherwise, we have nothing to do.
	return notStartedLegacyForgeHandshakeBackendPhase.handle(sc, msg)
}

// A special backend phase used to indicate that this connection is about to become
// obsolete (transfer to a new server, for instance) and that Forge messages ought to be
// forwarded on to an in-flight connection instead.
var inTransitionBackendPhase backendConnectionPhase = &inTransitionBackendPhase_{}

type inTransitionBackendPhase_ struct {
	noOpBackendPhase
}

func (inTransitionBackendPhase_) consideredComplete() bool {
	return true
}

//
//
//
//
//

type legacyForgeHandshakeBackendPhase struct {
	packetToAdvanceOn      *int                              // nil-able
	nextPhase              *legacyForgeHandshakeBackendPhase // nil-able
	onTransitionToNewPhase func(sc *serverConnection)        // nil-able: Performs any specific tasks when moving to a new phase.
	consideredComplete_    bool
}

var _ backendConnectionPhase = (*legacyForgeHandshakeBackendPhase)(nil)

var (
	// Indicates that the handshake has not started, used for unknownBackendPhase.
	notStartedLegacyForgeHandshakeBackendPhase = &legacyForgeHandshakeBackendPhase{
		packetToAdvanceOn: intPtr(forge.ServerHelloDiscriminator),
		nextPhase:         helloLegacyForgeHandshakeBackendPhase,
	}
	// Sent a hello to the client, waiting for a hello back before sending the mod list.
	helloLegacyForgeHandshakeBackendPhase = &legacyForgeHandshakeBackendPhase{
		packetToAdvanceOn:      intPtr(forge.ModListDiscriminator),
		nextPhase:              sendModListLegacyForgeHandshakeBackendPhase,
		onTransitionToNewPhase: nil, // see init()
	}
	// The mod list from the client has been accepted and a server mod list
	// has been sent. Waiting for the client to acknowledge.
	sendModListLegacyForgeHandshakeBackendPhase = &legacyForgeHandshakeBackendPhase{
		packetToAdvanceOn: intPtr(forge.ModListDiscriminator),
		nextPhase:         sendServerDataLegacyForgeHandshakeBackendPhase,
	}
	// The server data is being sent or has been sent, and is waiting for
	// the client to acknowledge it has processed this.
	sendServerDataLegacyForgeHandshakeBackendPhase = &legacyForgeHandshakeBackendPhase{
		packetToAdvanceOn: intPtr(forge.AckDiscriminator),
		nextPhase:         waitingAckLegacyForgeHandshakeBackendPhase,
	}
	// Waiting for the client to acknowledge before completing handshake.
	waitingAckLegacyForgeHandshakeBackendPhase = &legacyForgeHandshakeBackendPhase{
		packetToAdvanceOn: intPtr(forge.AckDiscriminator),
		nextPhase:         completeLegacyForgeHandshakeBackendPhase,
	}
	// The server has completed the handshake and will continue after the client ACK.
	completeLegacyForgeHandshakeBackendPhase = &legacyForgeHandshakeBackendPhase{
		consideredComplete_: true,
	}
)

func init() {
	helloLegacyForgeHandshakeBackendPhase.onTransitionToNewPhase = func(sc *serverConnection) {
		// We must always reset the handshake before a modded connection is established if
		// we haven't done so already.
		sc.mu.Lock()
		if sc.connection != nil {
			sc.connection.connType = LegacyForge
		}
		sc.mu.Unlock()
		sc.player.sendLegacyForgeHandshakeResetPacket()
	}
}

func (l *legacyForgeHandshakeBackendPhase) handle(sc *serverConnection, message *plugin.Message) bool {
	if strings.EqualFold(message.Channel, forge.LegacyHandshakeChannel) {
		// Get the phase and check if we need to start the next phase.
		newPhase := l.newPhase(sc, message)
		// Update phase on server
		sc.setConnectionPhase(newPhase)
		// Write the packet to the player, we don't need it now.
		if sc.player.WritePacket(message) == nil {
			return true
		}
	}
	// Not handled, fallback
	return false
}

func (l *legacyForgeHandshakeBackendPhase) consideredComplete() bool {
	return l.consideredComplete_
}

func (l *legacyForgeHandshakeBackendPhase) onDepartForNewServer(sc *serverConnection) {
	// If the server we are departing is modded, we must always reset the client's handshake.
	sc.player.phase().resetConnectionPhase(sc.player)
}

// Get the phase to act on, depending on the packet that has been sent.
// sc - the server the proxy is connecting to
// return - the phase to transition to, which may be the same as before
func (l *legacyForgeHandshakeBackendPhase) newPhase(sc *serverConnection, message *plugin.Message) *legacyForgeHandshakeBackendPhase {
	if l.packetToAdvanceOn != nil {
		discriminator, ok := forge.HandshakePacketDiscriminator(message)
		if ok && discriminator == byte(*l.packetToAdvanceOn) {

			phaseToTransitionTo := l.nextPhase
			if l.nextPhase == nil {
				phaseToTransitionTo = l // same phase if at end of handshake
			}

			if phaseToTransitionTo.onTransitionToNewPhase != nil {
				phaseToTransitionTo.onTransitionToNewPhase(sc)
			}
			return phaseToTransitionTo
		}
	}
	return l
}

func intPtr(i int) *int { return &i }

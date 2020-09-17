package proxy

import (
	"errors"
	"fmt"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/event"
	"go.uber.org/zap"
	"reflect"
)

type backendTransitionSessionHandler struct {
	serverConn    *serverConnection
	requestCtx    *connRequestCxt
	listenDoneCtx chan struct{}

	noOpSessionHandler
}

func newBackendTransitionSessionHandler(serverConn *serverConnection, requestCtx *connRequestCxt) sessionHandler {
	return &backendTransitionSessionHandler{serverConn: serverConn, requestCtx: requestCtx}
}

func (b *backendTransitionSessionHandler) activated() {
	b.listenDoneCtx = make(chan struct{})
	go func() {
		select {
		case <-b.listenDoneCtx:
		case <-b.requestCtx.Done():
			// We must check again since request context
			// may be canceled before deactivated() was run.
			select {
			case <-b.listenDoneCtx:
				return
			default:
				b.requestCtx.result(nil, errors.New(
					"context deadline exceeded while transitioning player to backend server"))
				b.serverConn.disconnect()
			}
		}
	}()
}

func (b *backendTransitionSessionHandler) deactivated() {
	if b.listenDoneCtx != nil {
		close(b.listenDoneCtx)
	}
}

func (b *backendTransitionSessionHandler) handlePacket(p proto.Packet) {
	if !b.shouldHandle() {
		return
	}
	switch t := p.(type) {
	case *packet.JoinGame:
		b.handleJoinGame(t)
	case *packet.KeepAlive:
		b.handleKeepAlive(t)
	case *packet.Disconnect:
		b.handleDisconnect(t)
	case *plugin.Message:
		b.handlePluginMessage(t)
	default:
		zap.L().Warn("Received unhandled packet from backend server while transitioning",
			zap.Stringer("type", reflect.TypeOf(p)))
	}
}

func (b *backendTransitionSessionHandler) shouldHandle() bool {
	if b.serverConn.active() {
		return true
	}
	// Obsolete connection
	b.serverConn.disconnect()
	return false
}

func (b *backendTransitionSessionHandler) handleKeepAlive(p *packet.KeepAlive) {
	_ = b.serverConn.conn().WritePacket(p)
}
func (b *backendTransitionSessionHandler) handleDisconnect(p *packet.Disconnect) {
	var connType connectionType
	b.serverConn.mu.Lock()
	if b.serverConn.connection != nil {
		connType = b.serverConn.connection.Type()
	}
	b.serverConn.disconnect0()
	b.serverConn.mu.Unlock()

	// If we were in the middle of the Forge handshake, it is not safe to proceed.
	// We must kick the client.
	safe := connType != LegacyForge || b.serverConn.phase().consideredComplete()
	result := disconnectResultForPacket(p, b.serverConn.player.Protocol(), b.serverConn.server, safe)
	b.requestCtx.result(result, nil)
}

func (b *backendTransitionSessionHandler) handlePluginMessage(packet *plugin.Message) {
	conn, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}
	if !b.serverConn.player.canForwardPluginMessage(conn.Protocol(), packet) {
		return
	}

	if plugin.Register(packet) {
		b.serverConn.player.pluginChannelsMu.Lock()
		b.serverConn.player.pluginChannels.Insert(plugin.Channels(packet)...)
		b.serverConn.player.pluginChannelsMu.Unlock()
	} else if plugin.Unregister(packet) {
		b.serverConn.player.pluginChannelsMu.Lock()
		b.serverConn.player.pluginChannels.Delete(plugin.Channels(packet)...)
		b.serverConn.player.pluginChannelsMu.Unlock()
	}

	// We always need to handle plugin messages, for Forge compatibility.
	if b.serverConn.phase().handle(b.serverConn, packet) {
		// Handled, but check the server connection phase.
		if b.serverConn.phase() == helloLegacyForgeHandshakeBackendPhase {

			phase, existingConn := func() (backendConnectionPhase, *serverConnection) {
				b.serverConn.player.mu.Lock()
				defer b.serverConn.player.mu.Unlock()

				existingConn := b.serverConn.player.connectedServer_
				if existingConn != nil && existingConn.connPhase != inTransitionBackendPhase {
					// Indicate that this connection is "in transition"
					existingConn.connPhase = inTransitionBackendPhase
					return existingConn.connPhase, existingConn
				}
				return nil, nil
			}()
			if phase != nil {
				// Tell the player that we're leaving and we just aren't coming back.
				phase.onDepartForNewServer(existingConn)
			}

		}
		return
	}

	_ = b.serverConn.player.Write(packet.Retained)
}

func (b *backendTransitionSessionHandler) handleJoinGame(p *packet.JoinGame) {
	smc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}

	failResult := func(format string, a ...interface{}) {
		err := fmt.Errorf(format, a...)
		zap.S().Errorf("Unable to switch %q to new server %q: %v",
			b.serverConn.player, b.serverConn.server.ServerInfo().Name(), err)
		b.serverConn.player.Disconnect(internalServerConnectionError)
		b.requestCtx.result(nil, err)
	}

	b.serverConn.player.mu.Lock()
	existingConn := b.serverConn.player.connectedServer_
	var previousServer RegisteredServer
	if existingConn != nil {
		previousServer = existingConn.server
		// Shut down the existing server connection.
		b.serverConn.player.connectedServer_ = nil
		b.serverConn.player.mu.Unlock()
		existingConn.disconnect()

		// Send keep alive to try to avoid timeouts
		if err := b.serverConn.player.SendKeepAlive(); err != nil {
			failResult("could not send keep alive packet, player might have disconnected: %v", err)
			return
		}
	} else {
		b.serverConn.player.mu.Unlock()
	}

	// The goods are in hand! We got JoinGame.
	// Let's transition completely to the new state.
	connectedEvent := &ServerConnectedEvent{
		player:         b.serverConn.player,
		server:         b.serverConn.server,
		previousServer: previousServer, // nil-able
	}
	// Fire event in same goroutine as we don't want to read
	// more incoming packets while we process the JoinGame!
	b.event().Fire(connectedEvent)
	// Make sure we can still transition,
	// event handler might have disconnected player.
	if !b.serverConn.player.Active() {
		failResult("player was disconnected")
		return
	}

	if previousServer == nil {
		zap.S().Infof("%s initial server %q", b.serverConn.player, b.serverConn.server.ServerInfo().Name())
	} else {
		zap.S().Infof("%s moved from %q to %q", b.serverConn.player, previousServer.ServerInfo().Name(),
			b.serverConn.server.ServerInfo().Name())
	}

	// Change client to use ClientPlaySessionHandler if required.
	b.serverConn.player.minecraftConn.mu.Lock()
	playHandler, ok := b.serverConn.player.minecraftConn.sessionHandler.(*clientPlaySessionHandler)
	if !ok {
		playHandler = newClientPlaySessionHandler(b.serverConn.player)
		b.serverConn.player.minecraftConn.setSessionHandler0(playHandler)
	}
	b.serverConn.player.minecraftConn.mu.Unlock()

	if !playHandler.handleBackendJoinGame(p, b.serverConn) {
		failResult("JoinGame packet could not be handled, client-side switching server failed")
		return // not handled
	}

	// Strap on the correct session handler for the server.
	// We will have nothing more to do with this connection once this task finishes up.
	backendPlay, err := newBackendPlaySessionHandler(b.serverConn)
	if err != nil {
		failResult("error creating backend player session handler: %v", err)
		return
	}
	smc.setSessionHandler(backendPlay)

	// Now set the connected server.
	b.serverConn.player.setConnectedServer(b.serverConn)

	// We're done!
	postConnectEvent := newServerPostConnectEvent(b.serverConn.player, previousServer)
	b.event().Fire(postConnectEvent)
	b.requestCtx.result(plainConnectionResult(SuccessConnectionStatus, b.serverConn.server), nil)
}

func (b *backendTransitionSessionHandler) disconnected() {
	b.requestCtx.result(nil, errors.New("unexpectedly disconnected from remote server"))
}

func (b *backendTransitionSessionHandler) event() *event.Manager {
	return b.serverConn.player.proxy.Event()
}

package proxy

import (
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/bungeecord"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/gate/proto"
	"reflect"
	"time"
)

type backendTransitionSessionHandler struct {
	eventMgr event.Manager

	serverConn                *serverConnection
	requestCtx                *connRequestCxt
	bungeeCordMessageRecorder bungeecord.MessageResponder
	listenDoneCtx             chan struct{}
	log                       logr.Logger
}

func newBackendTransitionSessionHandler(
	serverConn *serverConnection,
	requestCtx *connRequestCxt,
	proxy *Proxy,
) netmc.SessionHandler {
	return &backendTransitionSessionHandler{
		eventMgr:   proxy.Event(),
		serverConn: serverConn,
		requestCtx: requestCtx,
		bungeeCordMessageRecorder: newBungeeCordMessageResponder(
			serverConn.config().BungeePluginChannelEnabled,
			serverConn.player, proxy,
		),
		log: serverConn.log.WithName("backendTransitionSession")}
}

func (b *backendTransitionSessionHandler) Activated() {
	b.listenDoneCtx = make(chan struct{})
	go func() {
		select {
		case <-b.listenDoneCtx:
		case <-b.requestCtx.Done():
			// We must check again since request context
			// may be canceled before Deactivated() was run.
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

func (b *backendTransitionSessionHandler) Deactivated() {
	if b.listenDoneCtx != nil {
		close(b.listenDoneCtx)
	}
}

func (b *backendTransitionSessionHandler) HandlePacket(pc *proto.PacketContext) {
	if !pc.KnownPacket() {
		return // ignore unknown packet
	}

	if !b.shouldHandle() {
		return
	}
	switch p := pc.Packet.(type) {
	case *packet.JoinGame:
		b.handleJoinGame(pc, p)
	case *packet.KeepAlive:
		b.handleKeepAlive(p)
	case *packet.Disconnect:
		b.handleDisconnect(p)
	case *plugin.Message:
		b.handlePluginMessage(p)
	default:
		b.log.V(1).Info("Received unexpected packet from backend server while transitioning",
			"type", reflect.TypeOf(p))
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
	var connType phase.ConnectionType
	b.serverConn.mu.Lock()
	if b.serverConn.connection != nil {
		connType = b.serverConn.connection.Type()
	}
	b.serverConn.mu.Unlock()

	// If we were in the middle of the Forge handshake, it is not safe to proceed.
	// We must kick the client.
	safe := connType != phase.LegacyForge || b.serverConn.phase().ConsideredComplete()
	result := disconnectResultForPacket(b.log.V(1), p, b.serverConn.player.Protocol(), b.serverConn.server, safe)
	b.requestCtx.result(result, nil)

	b.serverConn.disconnect()
}

func (b *backendTransitionSessionHandler) handlePluginMessage(packet *plugin.Message) {
	if b.bungeeCordMessageRecorder.Process(packet) {
		return
	}

	// We always need to handle plugin messages, for Forge compatibility.
	player := b.serverConn.player
	backendConn := b.serverConn.conn()
	if b.serverConn.phase().Handle(player, b.serverConn, backendConn, player, packet) {
		// Handled, but check the server connection phase.
		if b.serverConn.phase() == phase.HelloLegacyForgeHandshakeBackendPhase {

			phase, _ := func() (phase.BackendConnectionPhase, *serverConnection) {
				player.mu.Lock()
				defer player.mu.Unlock()

				existingConn := player.connectedServer_
				if existingConn != nil && existingConn.connPhase != phase.InTransitionBackendPhase {
					// Indicate that this connection is "in transition"
					existingConn.connPhase = phase.InTransitionBackendPhase
					return existingConn.connPhase, existingConn
				}
				return nil, nil
			}()
			if phase != nil {
				// Tell the player that we're leaving and we just aren't coming back.
				phase.OnDepartForNewServer(player, player.phase(), player)
			}

		}
		return
	}

	_ = b.serverConn.player.WritePacket(packet)
}

func (b *backendTransitionSessionHandler) handleJoinGame(pc *proto.PacketContext, p *packet.JoinGame) {
	smc, ok := b.serverConn.ensureConnected()
	if !ok {
		return
	}

	failResult := func(format string, a ...any) {
		err := fmt.Errorf(format, a...)
		b.log.Error(err, "unable to switch player to new server, disconnecting")
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
		if err := netmc.SendKeepAlive(b.serverConn.player); err != nil {
			failResult("could not send keep alive packet, player might have disconnected: %w", err)
			return
		}

		// Reset Tablist header and footer to prevent desync
		if err := tablist.ClearHeaderFooter(b.serverConn.player); err != nil {
			failResult("could not clear tablist header and footer, player might have disconnected: %w", err)
			return
		}
	} else {
		b.serverConn.player.mu.Unlock()
	}

	// The goods are in hand! We got JoinGame.
	// Let's transition completely to the new state.
	smc.SetAutoReading(false)
	connectedEvent := &ServerConnectedEvent{
		player:         b.serverConn.player,
		server:         b.serverConn.server,
		previousServer: previousServer, // nil-able
		entityID:       p.EntityID,
	}
	// Fire event in same goroutine as we don't want to read
	// more incoming packets while we process the JoinGame!
	b.eventMgr.Fire(connectedEvent)
	// Make sure we can still transition,
	// event handler might have disconnected player.
	if !b.serverConn.player.Active() {
		failResult("player was disconnected")
		return
	}

	if previousServer == nil {
		b.log.Info("player joining initial server")
	} else {
		b.log.Info("player switching the server",
			"previous", previousServer.ServerInfo().Name(),
			"previousAddr", previousServer.ServerInfo().Addr())
	}

	var playHandler *clientPlaySessionHandler
	if smc.Protocol().Lower(version.Minecraft_1_20_2) {
		// Change client to use ClientPlaySessionHandler.
		playHandler, ok = b.serverConn.player.MinecraftConn.ActiveSessionHandler().(*clientPlaySessionHandler)
		if !ok { // should always be true
			playHandler = newClientPlaySessionHandler(b.serverConn.player)
			b.serverConn.player.MinecraftConn.SetActiveSessionHandler(state.Play, playHandler)
		}
	} else {
		// Wait for FinishedUpdate packet in clientConfigSessionHandler
		// to set the clientPlaySessionHandler.
		//
		// JoinGame from backend and FinishedUpdate from client might
		// arrive in any order, so we need to ensure that FinishedUpdate
		// is handled before JoinGame.
		const (
			maxWait = time.Second * 3
			tick    = time.Millisecond * 100
		)
		for waited := time.Duration(0); ; {
			if !b.serverConn.active() {
				failResult("server connection is no longer active")
				return
			}
			if waited >= maxWait {
				failResult("timed out waiting for client play session handler to be set")
				return
			}
			playHandler, ok = b.serverConn.player.MinecraftConn.ActiveSessionHandler().(*clientPlaySessionHandler)
			if ok {
				break
			}
			b.log.V(1).Info("waiting for client play session handler to be set")
			time.Sleep(tick)
			waited += tick
		}
	}

	if err := playHandler.handleBackendJoinGame(pc, p, b.serverConn); err != nil {
		failResult("JoinGame packet could not be handled, client-side switching server failed: %w", err)
		return // not handled
	}

	// Set the new play session handler for the server. We will have nothing more to do
	// with this connection once this task finishes up.
	backendPlay, err := newBackendPlaySessionHandler(b.serverConn)
	if err != nil {
		failResult("error creating backend play session handler: %w", err)
		return
	}
	smc.SetActiveSessionHandler(state.Play, backendPlay)

	// Now set the connected server.
	b.serverConn.player.setConnectedServer(b.serverConn)

	// Clean up disabling auto-read while the connected event was being processed.
	// Do this after setting the connection, so no incoming packets are processed before
	// the API knows which server the player is connected to.
	smc.SetAutoReading(true)

	// Send client settings. In 1.20.2+ this is done in the config state.
	if smc.Protocol().Lower(version.Minecraft_1_20_2) {
		if csp := b.serverConn.player.ClientSettingsPacket(); csp != nil {
			smc, ok = b.serverConn.ensureConnected()
			if ok {
				_ = smc.WritePacket(csp)
			}
		}
	}

	// We're done!
	postConnectEvent := newServerPostConnectEvent(b.serverConn.player, previousServer)
	b.eventMgr.Fire(postConnectEvent)
	b.requestCtx.result(plainConnectionResult(SuccessConnectionStatus, b.serverConn.server), nil)
}

func (b *backendTransitionSessionHandler) Disconnected() {
	b.requestCtx.result(nil, errors.New("unexpectedly disconnected from remote server"))
}

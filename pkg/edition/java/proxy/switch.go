package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/robinbraemer/event"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	util2 "go.minekube.com/gate/pkg/edition/java/proto/util"
)

// ConnectionRequest can send a connection request to another server on the proxy.
// A connection request is created using Player.CreateConnectionRequest(RegisteredServer).
type ConnectionRequest interface {
	// Server returns the server that this connection request is for.
	Server() RegisteredServer
	// Connect blocks, initiates the connection to the
	// remote Server and returns a result after the user has logged on
	// or an error when an error occurred (e.g. could not net.Dial the Server, ctx was canceled, etc.).
	//
	// The given Context can be used to cancel the connection initiation, but
	// has no effect if the connection was already established or canceled.
	//
	// No messages will be communicated to the client:
	// You are responsible for all error handling.
	Connect(ctx context.Context) (ConnectionResult, error)
	// ConnectWithIndication is the same as Connect, but the proxy's built-in
	// handling will be used to provide errors to the player and returns
	// true if the player was successfully connected.
	ConnectWithIndication(ctx context.Context) (successful bool)
}

// ConnectionResult is the result of a ConnectionRequest.
type ConnectionResult interface {
	Status() ConnectionStatus // The connection result status.
	// Reason returns a reason for the failure to connect to the server.
	// It is nil if not provided.
	Reason() Component
}

// ConnectionStatus is the status for a ConnectionResult
type ConnectionStatus uint8

const (
	// SuccessConnectionStatus indicates that the player was successfully connected to the server.
	SuccessConnectionStatus ConnectionStatus = iota
	// AlreadyConnectedConnectionStatus indicates that the player is already connected to this server.
	AlreadyConnectedConnectionStatus
	// InProgressConnectionStatus indicates that a connection is already in progress.
	InProgressConnectionStatus
	// CanceledConnectionStatus indicates that a plugin has cancelled this connection.
	CanceledConnectionStatus
	// ServerDisconnectedConnectionStatus indicates that the server disconnected the player.
	// A reason MAY be provided in the ConnectionResult.Reason().
	ServerDisconnectedConnectionStatus
)

// Successful is true if the player was successfully connected to the server.
func (r ConnectionStatus) Successful() bool {
	return r == SuccessConnectionStatus
}

// AlreadyConnected id true if the player is already connected to this server.
func (r ConnectionStatus) AlreadyConnected() bool {
	return r == AlreadyConnectedConnectionStatus
}

// ConnectionInProgress is true if a connection is already in progress.
func (r ConnectionStatus) ConnectionInProgress() bool {
	return r == InProgressConnectionStatus
}

// Canceled is true if a plugin has cancelled this connection.
func (r ConnectionStatus) Canceled() bool {
	return r == CanceledConnectionStatus
}

// ServerDisconnected is true if the server disconnected the player.
// A reason MAY be provided in the ConnectionResult.Reason().
func (r ConnectionStatus) ServerDisconnected() bool {
	return r == ServerDisconnectedConnectionStatus
}

//
//
//
//
//
//
//

func (p *connectedPlayer) CreateConnectionRequest(server RegisteredServer) ConnectionRequest {
	return p.createConnectionRequest(server)
}

func (p *connectedPlayer) createConnectionRequest(server RegisteredServer) *connectionRequest {
	return &connectionRequest{server: server, player: p}
}

type connectionRequest struct {
	server RegisteredServer // the target server to connect to
	player *connectedPlayer // the player to connect to the server
}

func (c *connectionRequest) connect(ctx context.Context) (*connectionResult, error) {
	result, err := c.internalConnect(ctx)
	if err == nil {
		if !result.safe {
			// It's not safe to continue the connection, we need to shut it down.
			c.player.handleConnectionErr(result.attemptedConn, err, true)
		} else if !result.Status().Successful() {
			c.player.resetInFlightConnection()
		}
	}
	return result, err
}

// Connect - See ConnectionRequest interface.
func (c *connectionRequest) Connect(ctx context.Context) (ConnectionResult, error) {
	return c.connect(ctx)
}

// ConnectWithIndication - See ConnectionRequest interface.
func (c *connectionRequest) ConnectWithIndication(ctx context.Context) (successful bool) {
	result, err := c.internalConnect(ctx)
	if err != nil {
		c.player.handleConnectionErr(c.server, err, true)
		return false
	}

	switch result.Status() {
	case AlreadyConnectedConnectionStatus:
		_ = c.player.SendMessage(alreadyConnected)
	case InProgressConnectionStatus:
		_ = c.player.SendMessage(alreadyInProgress)
	case CanceledConnectionStatus:
		// Ignore, event subscriber probably handled this.
	case ServerDisconnectedConnectionStatus:
		reason := result.Reason()
		if reason == nil {
			reason = internalServerConnectionError
		}
		c.player.handleDisconnectWithReason(c.server, reason, result.safe)
	default:
		// The only remaining value is successful (no need to do anything!)
	}

	return result.Status().Successful()
}

// Handles unexpected disconnects.
// server - the server we disconnected from
// safe - whether we can safely reconnect to a new server
func (p *connectedPlayer) handleConnectionErr(server RegisteredServer, err error, safe bool) {
	log := p.log.WithValues(
		"serverName", server.ServerInfo().Name(),
		"serverAddr", server.ServerInfo().Addr())
	log.V(1).Info("could not connect player to server", "error", err)

	errorEvent := newConnectionErrorEvent(err, safe, p, server)
	p.eventMgr.Fire(errorEvent)

	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	var userMsg string
	connectedServer := p.CurrentServer()
	if connectedServer != nil && RegisteredServerEqual(connectedServer.Server(), server) {
		userMsg = fmt.Sprintf("Your connection to %q encountered an error.",
			server.ServerInfo().Name())
	} else {
		log.Info("unable to connect to server", "error", err)
		userMsg = fmt.Sprintf("Unable to connect to %q. Try again later.", server.ServerInfo().Name())
	}
	p.handleConnectionErr2(server, nil, &Text{Content: userMsg, S: Style{Color: Red}}, safe)
}

func (p *connectedPlayer) handleConnectionErr2(
	rs RegisteredServer,
	kickReason Component,
	friendlyReason Component,
	safe bool,
) {
	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}
	if !safe {
		// /!\ IT IS UNSAFE TO CONTINUE /!\
		//
		// This is usually triggered by a failed Forge handshake.
		p.Disconnect(friendlyReason)
		return
	}
	currentServer := p.CurrentServer()
	kickedFromCurrent := currentServer == nil || RegisteredServerEqual(currentServer.Server(), rs)
	var result ServerKickResult
	if kickedFromCurrent {
		next := p.nextServerToTry(rs)
		if next == nil {
			result = &DisconnectPlayerKickResult{Reason: friendlyReason}
		} else {
			result = &RedirectPlayerKickResult{Server: next}
		}
	} else {
		// If we were kicked by going to another server, the connection should not be in flight
		p.mu.Lock()
		if p.connInFlight != nil && RegisteredServerEqual(p.connInFlight.Server(), rs) {
			p.resetInFlightConnection0()
		}
		p.mu.Unlock()
		result = &NotifyKickResult{Message: friendlyReason}
	}
	e := newKickedFromServerEvent(p, rs, kickReason, !kickedFromCurrent, result)
	p.handleKickEvent(e, friendlyReason, kickedFromCurrent)
}

func (p *connectedPlayer) handleKickEvent(e *KickedFromServerEvent, friendlyReason Component, kickedFromCurrent bool) {
	p.proxy.Event().Fire(e)

	// There can't be any connection in flight now.
	p.setInFlightConnection(nil)

	// Make sure we clear the current connected server as the connection is invalid.
	p.mu.Lock()
	previouslyConnected := p.connectedServer_ != nil
	if kickedFromCurrent {
		p.connectedServer_ = nil
	}
	p.mu.Unlock()

	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	switch result := e.Result().(type) {
	case *DisconnectPlayerKickResult:
		p.Disconnect(result.Reason)
	case *RedirectPlayerKickResult:
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.config().ConnectionTimeout)*time.Millisecond)
		defer cancel()
		redirect, err := p.createConnectionRequest(result.Server).connect(ctx)
		if err != nil {
			p.handleConnectionErr(result.Server, err, true)
			return
		}

		switch redirect.Status() {
		// Impossible/nonsensical cases
		case AlreadyConnectedConnectionStatus, InProgressConnectionStatus:
		// Fatal case
		case CanceledConnectionStatus:
			reason := redirect.Reason()
			if reason == nil {
				reason = result.Message
			}
			if reason == nil {
				reason = friendlyReason
			}
			p.Disconnect(reason)
		case ServerDisconnectedConnectionStatus:
			reason := redirect.Reason()
			if reason == nil {
				reason = internalServerConnectionError
			}
			p.handleDisconnectWithReason(result.Server, reason, redirect.safe)
		case SuccessConnectionStatus:
			requestedMessage := result.Message
			if requestedMessage == nil {
				requestedMessage = friendlyReason
			}
			_ = p.SendMessage(requestedMessage)
		}
	case *NotifyKickResult:
		if e.KickedDuringServerConnect() && previouslyConnected {
			_ = p.SendMessage(result.Message)
		} else {
			p.Disconnect(result.Message)
		}
	default:
		// In case someone gets creative, assume we want to disconnect the player.
		p.Disconnect(friendlyReason)
	}
}

func (p *connectedPlayer) handleDisconnect(server RegisteredServer, disconnect *packet.Disconnect, safe bool) {
	p.handleDisconnectWithReason(server, disconnect.Reason.AsComponentOrNil(), safe)
}

// handles unexpected disconnects
func (p *connectedPlayer) handleDisconnectWithReason(server RegisteredServer, reason Component, safe bool) {
	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	log := p.log.WithValues("server", server.ServerInfo().Name())

	if plainReason, err := util2.MarshalPlain(reason); err != nil {
		p.log.V(1).Info("error marshal disconnect reason to plain", "error", err)
	} else {
		log = log.WithValues("reason", plainReason)
	}

	connected := p.connectedServer()
	if connected != nil && ServerInfoEqual(connected.server.ServerInfo(), server.ServerInfo()) {
		log.Info("player was kicked from server")
		p.handleConnectionErr2(server, reason, &Text{
			Content: movedToNewServer.Content,
			S:       movedToNewServer.S,
			Extra:   []Component{reason},
		}, safe)
		return
	}

	log.Info("player disconnected from server while connecting")
	p.handleConnectionErr2(server, reason, &Text{
		Content: fmt.Sprintf("Can't connect to server %q: ", server.ServerInfo().Name()),
		S:       Style{Color: Red},
		Extra:   []Component{reason},
	}, safe)
}

func (p *connectedPlayer) resetInFlightConnection() {
	p.setInFlightConnection(nil)
}

// without locking
func (p *connectedPlayer) resetInFlightConnection0() {
	p.connInFlight = nil
}

func (p *connectedPlayer) setInFlightConnection(s *serverConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connInFlight = s
}

func (c *connectionRequest) Server() RegisteredServer {
	return c.server
}

func (c *connectionRequest) checkServer(server RegisteredServer) (s ConnectionStatus, ok bool) {
	p := c.player
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.connInFlight != nil || (p.connectedServer_ != nil &&
		!p.connectedServer_.completedJoin.Load()) {
		return InProgressConnectionStatus, false
	}
	if p.connectedServer_ != nil && RegisteredServerEqual(p.connectedServer_.Server(), server) {
		return AlreadyConnectedConnectionStatus, false
	}
	return 0, true
}

func (c *connectionRequest) internalConnect(ctx context.Context) (result *connectionResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	status, ok := c.checkServer(c.server)
	if !ok {
		return plainConnectionResult(status, c.server), nil
	}

	connectEvent := newServerPreConnectEvent(c.player, c.server)
	c.event().Fire(connectEvent)
	if !connectEvent.Allowed() {
		return plainConnectionResult(CanceledConnectionStatus, c.server), nil
	}

	newDest := connectEvent.Server()
	if newDest == nil {
		return plainConnectionResult(CanceledConnectionStatus, newDest), nil
	}
	status, ok = c.checkServer(newDest)
	if !ok {
		return plainConnectionResult(status, newDest), nil
	}

	server, ok := newDest.(*registeredServer)
	if !ok { // Must be of this type
		return plainConnectionResult(CanceledConnectionStatus, newDest), nil
	}

	conn := newServerConnection(server, c.player)
	c.player.setInFlightConnection(conn)
	defer c.resetIfInFlightIs(conn)
	return conn.connect(ctx)
}

func (c *connectionRequest) resetIfInFlightIs(establishedConnection *serverConnection) {
	c.player.mu.Lock()
	defer c.player.mu.Unlock()
	if c.player.connInFlight == establishedConnection {
		c.player.connInFlight = nil
	}
}

func plainConnectionResult(status ConnectionStatus, attemptedConn RegisteredServer) *connectionResult {
	return &connectionResult{
		status:        status,
		safe:          true,
		attemptedConn: attemptedConn,
	}
}

func (c *connectionRequest) event() event.Manager {
	return c.player.proxy.event
}

//
//
//
//
//
//

type connectionResult struct {
	status        ConnectionStatus
	reason        Component
	safe          bool
	attemptedConn RegisteredServer
}

func (r *connectionResult) Status() ConnectionStatus {
	return r.status
}

func (r *connectionResult) Reason() Component {
	return r.reason
}

var _ ConnectionResult = (*connectionResult)(nil)

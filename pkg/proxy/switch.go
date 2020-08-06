package proxy

import (
	"context"
	"fmt"
	. "go.minekube.com/common/minecraft/color"
	. "go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec"
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/util"
	"go.uber.org/zap"
	"strings"
)

// ConnectionRequest can send a connection request to another server on the proxy.
// A connection request is created using Player.CreateConnectionRequest(RegisteredServer).
type ConnectionRequest interface {
	// Returns the server that this connection request is for.
	Server() RegisteredServer
	// This method is blocking, initiates the connection to the
	// remote Server and returns a result after the user has logged on
	// or an error when an error occurred (e.g. could not net.Dial the Server, ctx was called, etc.).
	//
	// The given Context can be used to cancel the connection initiation, but
	// has no effect if the connection was already established or canceled.
	//
	// No messages will be communicated to the client:
	// You are responsible for all error handling.
	Connect(ctx context.Context) (ConnectionResult, error)
	// This method is the same as Connect, but the proxy's built-in
	// handling will be used to provide errors to the player and returns
	// true if the player was successfully connected.
	ConnectWithIndication(ctx context.Context) (successful bool)
}

// ConnectionResult is the result of a ConnectionRequest.
type ConnectionResult interface {
	Status() ConnectionStatus // The connection result status.
	// May be nil!
	Reason() Component // Returns a reason for the failure to connect to the server.
}

type ConnectionResultFn func(ConnectionResult, error)

// ConnectionStatus is the status for a ConnectionResult
type ConnectionStatus uint8

const (
	// The player was successfully connected to the server.
	SuccessConnectionStatus ConnectionStatus = iota
	// The player is already connected to this server.
	AlreadyConnectedConnectionStatus
	// A connection is already in progress.
	InProgressConnectionStatus
	// A plugin has cancelled this connection.
	CanceledConnectionStatus
	// The server disconnected the player.
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
	return &connectionRequest{server: server, player: p}
}

type connectionRequest struct {
	server RegisteredServer // the target server to connect to
	player *connectedPlayer // the player to connect to the server
}

func (c *connectionRequest) Connect(ctx context.Context) (ConnectionResult, error) {
	type res struct {
		ConnectionResult
		error
	}
	resultChan := make(chan *res, 1)
	c.internalConnect(ctx, func(result *connectionResult, err error) {
		if err == nil {
			if !result.safe {
				// If it's not safe to continue the connection we need to shut it down.
				c.player.handleConnectionErr(result.attemptedConn, err, true)
			} else if !result.Status().Successful() {
				c.player.resetInFlightConnection()
			}
		}
		resultChan <- &res{result, err}
	})
	r := <-resultChan
	return r.ConnectionResult, r.error
}

func (c *connectionRequest) ConnectWithIndication(ctx context.Context) (successful bool) {
	resultChan := make(chan bool, 1)
	c.internalConnect(ctx, func(result *connectionResult, err error) {
		if err != nil {
			c.player.handleConnectionErr(c.server, err, true)
			resultChan <- false
			return
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
			c.player.handleConnectionErr1(c.server, reason, result.safe)
		default:
			// The only remaining value is successful (no need to do anything!)
		}
		resultChan <- result.Status().Successful()
	})
	return <-resultChan
}

// Handles unexpected disconnects.
// server - the server we disconnected from
// safe - whether or not we can safely reconnect to a new server
func (p *connectedPlayer) handleConnectionErr(server RegisteredServer, err error, safe bool) {
	zap.L().Debug("Could not connect player to server",
		zap.String("server", server.ServerInfo().Name()),
		zap.String("addr", server.ServerInfo().Addr().String()),
		zap.Error(err))

	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	connectedServer := p.CurrentServer()
	// TODO use localization
	var userMsg string
	if connectedServer != nil && connectedServer.Server().Equals(server) {
		userMsg = fmt.Sprintf("Your connection to %q encountered an error.",
			server.ServerInfo().Name())
	} else {
		zap.L().Info("unable to connect to server",
			zap.String("serverName", server.ServerInfo().Name()),
			zap.String("serverAddr", server.ServerInfo().Addr().String()),
			zap.String("playerName", p.Username()),
			zap.Error(err),
		)
		userMsg = fmt.Sprintf("Unable to connect to %q. Try again later.", server.ServerInfo().Name())
	}
	p.handleConnectionErr2(server, nil, &Text{Content: userMsg, S: Style{Color: Red}}, safe)
}

func (p *connectedPlayer) handleConnectionErr1(
	server RegisteredServer,
	disconnectReason Component,
	safe bool,
) {
	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	b := new(strings.Builder)
	_ = (&codec.Plain{}).Marshal(b, disconnectReason)
	plainReason := b.String()

	connectedServer := p.CurrentServer()
	if connectedServer != nil && connectedServer.Server().Equals(server) {
		zap.S().Error("%s: kicked from server %s: %s", p, server.ServerInfo().Name(), plainReason)
		p.handleConnectionErr2(server, disconnectReason, &Text{
			Content: fmt.Sprintf("Kicked from %q: ", server.ServerInfo().Name()),
			S:       Style{Color: Red},
		}, safe)
	}

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
	kickedFromCurrent := currentServer == nil || currentServer.Server().Equals(rs)
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
		if p.connInFlight != nil && p.connInFlight.Server().Equals(rs) {
			p.resetInFlightConnection0()
		}
		p.mu.Unlock()
		result = &NotifyKickResult{Message: friendlyReason}
	}
	e := newKickedFromServerEvent(p, rs, kickReason, !kickedFromCurrent, result)
	p.handleKickEvent(e, friendlyReason)
}

func (p *connectedPlayer) handleKickEvent(e *KickedFromServerEvent, friendlyReason Component) {
	p.proxy.Event().Fire(e)

	// There can't be any connection in flight now.
	p.setInFlightConnection(nil)

	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	switch result := e.Result().(type) {
	case *DisconnectPlayerKickResult:
		p.Disconnect(result.Reason)
	case *RedirectPlayerKickResult:
		successful := p.CreateConnectionRequest(result.Server).ConnectWithIndication(context.Background())
		if successful {
			if result.Message == nil {
				_ = p.SendMessage(movedToNewServer)
			} else {
				_ = p.SendMessage(result.Message)
			}
		} else {
			p.Disconnect(friendlyReason)
		}
	case *NotifyKickResult:
		if e.KickedDuringServerConnect() {
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
	if !p.Active() {
		// If the connection is no longer active, we don't have to try recover it.
		return
	}

	reason, _ := util.JsonCodec(p.Protocol()).Unmarshal([]byte(*disconnect.Reason))
	b := new(strings.Builder)
	_ = (&codec.Plain{}).Marshal(b, reason)
	plainReason := b.String()

	connected := p.connectedServer()
	if connected != nil && connected.server.ServerInfo().Equals(server.ServerInfo()) {
		zap.S().Infof("%s was kicked from server %q: %s", p, server.ServerInfo().Name(), plainReason)
		p.handleConnectionErr2(server, reason, &Text{
			Content: fmt.Sprintf("Kicked from %q: ", server.ServerInfo().Name()),
			S:       Style{Color: Red},
			Extra:   []Component{reason},
		}, safe)
		return
	}

	zap.S().Errorf("%s disconnected while connecting to %q: %s", p, server.ServerInfo().Name(), plainReason)
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
	if p.connectedServer_ != nil && p.connectedServer_.Server().Equals(server) {
		return AlreadyConnectedConnectionStatus, false
	}
	return 0, true
}

type internalConnectionResultFn func(result *connectionResult, err error)

func (c *connectionRequest) internalConnect(ctx context.Context, resultFn internalConnectionResultFn) {
	if ctx == nil {
		ctx = context.Background()
	}
	if resultFn == nil {
		resultFn = func(result *connectionResult, err error) {}
	}

	status, ok := c.checkServer(c.server)
	if !ok {
		resultFn(plainConnectionResult(status, c.server), nil)
		return
	}

	connectEvent := newServerPreConnectEvent(c.player, c.server)
	c.event().Fire(connectEvent)
	if !connectEvent.Allowed() {
		resultFn(plainConnectionResult(CanceledConnectionStatus, c.server), nil)
		return
	}

	newDest := connectEvent.Server()
	status, ok = c.checkServer(newDest)
	if !ok {
		resultFn(plainConnectionResult(status, newDest), nil)
		return
	}

	server, ok := newDest.(*registeredServer)
	if !ok { // Must be of this type
		resultFn(plainConnectionResult(CanceledConnectionStatus, newDest), nil)
		return
	}

	con := newServerConnection(server, c.player)
	// TODO goroutine: register in flight connection context to be gracefully canceled on Proxy shutdown?
	c.player.setInFlightConnection(con)
	con.connect(ctx, func(result *connectionResult, err error) {
		c.resetIfInFlightIs(con)
		resultFn(result, err)
	})
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

func (c *connectionRequest) event() *event.Manager {
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

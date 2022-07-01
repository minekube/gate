package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/forge"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Players is a list of players safe for concurrent use.
type Players interface {
	Len() int                  // Returns the size of the player list.
	Range(func(p Player) bool) // Loops through the players, breaks if func returns false.
}

type players struct {
	mu   sync.RWMutex // Protects following fields
	list map[uuid.UUID]*connectedPlayer
}

func newPlayers() *players {
	return &players{list: map[uuid.UUID]*connectedPlayer{}}
}

// Len returns the size of the players list
func (p *players) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.list)
}

// Range loops through the player list.
func (p *players) Range(fn func(p Player) bool) {
	p.mu.RLock()
	list := p.list
	p.mu.RUnlock()
	for _, player := range list {
		if !fn(player) {
			return
		}
	}
}

func (p *players) add(players ...*connectedPlayer) {
	p.mu.Lock()
	for _, player := range players {
		p.list[player.ID()] = player
	}
	p.mu.Unlock()
}

func (p *players) remove(players ...*connectedPlayer) {
	p.mu.Lock()
	for _, player := range players {
		delete(p.list, player.ID())
	}
	p.mu.Unlock()
}

//
//
//
//
//
//
//
//
//
//
//

// ServerInfo is the info of a backend server.
type ServerInfo interface {
	Name() string   // Returns the server name.
	Addr() net.Addr // Returns the server address.
}

func NewServerInfo(name string, addr net.Addr) ServerInfo {
	return &serverInfo{name: name, addr: addr}
}

// ServerInfoEqual returns true if ServerInfo a and b are equal.
// They are never equal if one of them is nil.
func ServerInfoEqual(a, b ServerInfo) bool {
	return a != nil && b != nil &&
		a.Name() == b.Name() &&
		a.Addr().String() == b.Addr().String() &&
		a.Addr().Network() == b.Addr().Network()
}

type serverInfo struct {
	name string
	addr net.Addr
}

func (i *serverInfo) Name() string {
	return i.name
}

func (i *serverInfo) Addr() net.Addr {
	return i.addr
}

func (i *serverInfo) String() string { return fmt.Sprintf("%s (%s)", i.name, i.addr.String()) }

//
//
//
//
//
//
//
//
//
//

// RegisteredServer is a backend server that has been registered with the proxy.
type RegisteredServer interface {
	ServerInfo() ServerInfo
	Players() Players             // The players connected to the server on THIS proxy.
	Equals(RegisteredServer) bool // TODO remove
}

// RegisteredServerEqual returns true if RegisteredServer a and b are equal.
// They are never equal if one of them is nil.
func RegisteredServerEqual(a, b RegisteredServer) bool {
	return a != nil && b != nil && ServerInfoEqual(a.ServerInfo(), b.ServerInfo())
}

type registeredServer struct {
	info    ServerInfo
	players *players
}

func newRegisteredServer(info ServerInfo) *registeredServer {
	return &registeredServer{info: info, players: newPlayers()}
}

// TODO remove
func (r *registeredServer) Equals(o RegisteredServer) bool { return RegisteredServerEqual(r, o) }

func (r *registeredServer) ServerInfo() ServerInfo {
	return r.info
}

func (r *registeredServer) Players() Players {
	return r.players
}

var _ RegisteredServer = (*registeredServer)(nil)

// sends the a plugin message to all players on this server.
func (r *registeredServer) sendPluginMessage(identifier message.ChannelIdentifier, data []byte) {
	if r == nil {
		return
	}
	r.Players().Range(func(p Player) bool {
		go func(p Player) { _ = p.SendPluginMessage(identifier, data) }(p)
		return true
	})
}

//
//
//
//
//
//
//
//
//

// ServerConnection is a connection to a backend server from the proxy for a client.
type ServerConnection interface {
	message.ChannelMessageSink
	message.ChannelMessageSource

	Server() RegisteredServer // Returns the server that this connection is connected to.
	Player() Player           // Returns the player that this connection is associated with.
}

type serverConnection struct {
	server *registeredServer
	player *connectedPlayer
	log    logr.Logger

	completedJoin           atomic.Bool
	gracefulDisconnect      atomic.Bool
	lastPingID              atomic.Int64
	lastPingSent            atomic.Int64              // unix millis
	activeDimensionRegistry *packet.DimensionRegistry // updated by packet.JoinGame

	mu         sync.RWMutex   // Protects following fields
	connection *minecraftConn // the backend server connection
	connPhase  backendConnectionPhase
}

func newServerConnection(server *registeredServer, player *connectedPlayer) *serverConnection {
	return &serverConnection{server: server, player: player,
		log: player.log.WithName("serverConn").WithValues(
			"serverName", server.info.Name(),
			"serverAddr", server.info.Addr()),
	}
}

var _ ServerConnection = (*serverConnection)(nil)

// returns the backend server connection, nil-able
func (s *serverConnection) conn() *minecraftConn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connection
}

func (s *serverConnection) SendPluginMessage(id message.ChannelIdentifier, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if id == nil {
		return errors.New("identifier must not be nil")
	}
	mc, ok := s.ensureConnected()
	if !ok {
		return ErrClosedConn
	}
	return mc.WritePacket(&plugin.Message{
		Channel: id.ID(),
		Data:    data,
	})
}

func (s *serverConnection) Server() RegisteredServer {
	return s.server
}

func (s *serverConnection) Player() Player {
	return s.player
}

func (s *serverConnection) setConnectionPhase(phase backendConnectionPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connPhase = phase
}
func (s *serverConnection) phase() backendConnectionPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connPhase
}

type (
	connRequestCxt struct {
		context.Context
		response chan<- *connResponse
		once     sync.Once
	}
	connResponse struct {
		*connectionResult
		error
	}
)

func (c *connRequestCxt) result(result *connectionResult, err error) {
	c.once.Do(func() { c.response <- &connResponse{connectionResult: result, error: err} })
}

// ServerDialer provides the server connection for a joining player.
// A ServerInfo of a registered server or a RegisteredServer can implement this interface
// to provide custom connection establishment when a player wants to join a server.
// If no ServerInfo or RegisteredServer implements this interface the ServerInfo.Addr is
// used to dial the server using tcp.
type ServerDialer interface {
	Dial(ctx context.Context, player Player) (net.Conn, error)
}

func (s *serverConnection) dial(ctx context.Context) (net.Conn, error) {
	var (
		sd ServerDialer
		ok bool
	)
	if sd, ok = s.Server().ServerInfo().(ServerDialer); !ok {
		if sd, ok = s.Server().(ServerDialer); !ok {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", s.Server().ServerInfo().Addr().String())
		}
	}
	return sd.Dial(ctx, s.player)
}

// HandshakeAddresser provides the ServerAddress sent with the packet.Handshake when a player joins the server
// implementing this interface.
// A ServerInfo of a registered server or a RegisteredServer can implement this interface.
// If no ServerInfo or RegisteredServer implements this interface the ServerInfo.Addr the default ServerAddress is used
// or the BungeeCord forwarding scheme if the proxy is in config.LegacyForwardingMode.
type HandshakeAddresser interface {
	HandshakeAddr(defaultPlayerVirtualHost string, player Player) (newPlayerVirtualHost string)
}

func (s *serverConnection) handshakeAddr(vHost string, player Player) string {
	var ha HandshakeAddresser
	var ok bool
	if ha, ok = s.Server().ServerInfo().(HandshakeAddresser); !ok {
		if ha, ok = s.Server().(HandshakeAddresser); !ok {
			if s.config().Forwarding.Mode == config.LegacyForwardingMode {
				return s.createLegacyForwardingAddress()
			}
		}
	}
	if ha != nil {
		vHost = ha.HandshakeAddr(vHost, player)
	}
	if s.player.Type() == LegacyForge {
		vHost += forge.HandshakeHostnameToken
	}
	return vHost
}

func (s *serverConnection) connect(ctx context.Context) (result *connectionResult, err error) {
	// Connect proxy -> server
	debug := s.log.V(1)
	debug.Info("Connecting to server...")
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to server %q: %w", s.server.ServerInfo().Name(), err)
	}
	debug.Info("Connected to server")

	// Wrap server connection
	serverMc := newMinecraftConn(conn, s.player.proxy, false)
	resultChan := make(chan *connResponse, 1)
	serverMc.setSessionHandler0(newBackendLoginSessionHandler(s, &connRequestCxt{
		Context:  ctx,
		response: resultChan,
	}))

	// Update serverConnection
	s.mu.Lock()
	s.connection = serverMc
	s.connPhase = serverMc.connType.initialBackendPhase()
	s.mu.Unlock()

	debug.Info("Establishing player connection with server...")

	// Initiate the handshake.
	protocol := s.player.Protocol()
	handshake := &packet.Handshake{
		ProtocolVersion: int(protocol),
		NextStatus:      int(state.LoginState),
		Port:            int16(netutil.Port(s.server.ServerInfo().Addr())),
	}

	// Set handshake ServerAddress
	{
		playerVHost := netutil.Host(s.player.virtualHost)
		if playerVHost == "" {
			playerVHost = netutil.Host(s.server.ServerInfo().Addr())
		}
		handshake.ServerAddress = s.handshakeAddr(playerVHost, s.player)
	}
	if err = serverMc.BufferPacket(handshake); err != nil {
		return nil, fmt.Errorf("error buffer handshake packet in server connection: %w", err)
	}

	// Set server's protocol & state
	// after writing handshake, but before writing ServerLogin
	serverMc.setProtocol(protocol)
	serverMc.setState(state.Login)

	// Kick off the connection process
	// connection from proxy -> server (backend)
	err = serverMc.WritePacket(&packet.ServerLogin{
		Username:  s.player.Username(),
		PlayerKey: s.player.IdentifiedKey(),
	})
	if err != nil {
		return nil, fmt.Errorf("error writing ServerLogin packet to server connection: %w", err)
	}
	go serverMc.readLoop()

	// Block
	r := <-resultChan
	return r.connectionResult, r.error
}

func (s *serverConnection) createLegacyForwardingAddress() string {
	// BungeeCord IP forwarding is simply a special injection after the "address" in the handshake,
	// separated by \0 (the null byte). In order, you send the original host, the player's IP, their
	// ID (undashed), and if you are in online-mode, their login properties (from Mojang).
	playerIP := netutil.Host(s.player.RemoteAddr())
	b := new(strings.Builder)
	b.WriteString(s.server.ServerInfo().Addr().String())
	b.WriteString("\000")
	b.WriteString(playerIP)
	b.WriteString("\000")
	b.WriteString(s.player.profile.ID.Undashed())
	b.WriteString("\000")
	props, err := json.Marshal(s.player.profile.Properties)
	if err != nil { // should never happen
		panic(err)
	}
	b.WriteString(string(props)) // first convert props to string
	return b.String()
}

// Returns the active backend server connection or false if inactive.
func (s *serverConnection) ensureConnected() (backend *minecraftConn, connected bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connection, s.connection != nil
}

// Ensures that this server connection remains "active": the connection is established and not
// closed, the player is still connected to the server, and the player still remains online.
func (s *serverConnection) active() bool {
	s.mu.RLock()
	conn := s.connection
	s.mu.RUnlock()
	return conn != nil && !conn.Closed() &&
		!s.gracefulDisconnect.Load() &&
		s.player.Active()
}

// disconnects from the server
func (s *serverConnection) disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnect0()
}
func (s *serverConnection) disconnect0() {
	if s.connection != nil {
		s.gracefulDisconnect.Store(true)
		if !s.connection.Closed() { // only close if not already closing to prevent deadlock
			_ = s.connection.closeKnown(false)
		}
		s.connection = nil // nil means not connected
	}
}

// Indicates that we have completed the plugin process.
func (s *serverConnection) completeJoin() {
	if s.completedJoin.CAS(false, true) {
		s.mu.Lock()
		if s.connPhase == unknownBackendPhase {
			// Now we know
			s.connPhase = vanillaBackendPhase
			if s.connection != nil {
				s.connection.setType(vanillaConnectionType)
			}
		}
		s.mu.Unlock()
	}
}

func (s *serverConnection) config() *config.Config {
	return s.player.proxy.config
}

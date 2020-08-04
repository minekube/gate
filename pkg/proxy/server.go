package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/proto"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/proto/state"
	"go.minekube.com/gate/pkg/proxy/forge"
	"go.minekube.com/gate/pkg/proxy/message"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"net"
	"strconv"
	"strings"
	"sync"
)

// RegisteredServer is a backend server that has been registered with the proxy.
type RegisteredServer interface {
	ServerInfo() ServerInfo
	Players() Players // The players connected to the server on THIS proxy.
	//TODO Ping() (*ServerPing, error)
	Equals(RegisteredServer) bool
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
//

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
		p.list[player.Id()] = player
	}
	p.mu.Unlock()
}

func (p *players) remove(players ...*connectedPlayer) {
	p.mu.Lock()
	for _, player := range players {
		delete(p.list, player.Id())
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
	Equals(ServerInfo) bool
}

type serverInfo struct {
	name string
	addr net.Addr
}

func (i *serverInfo) Equals(o ServerInfo) bool {
	return i == nil && o == nil || (i != nil && o != nil &&
		i.Name() == o.Name() &&
		i.Addr().String() == o.Addr().String() &&
		i.Addr().Network() == o.Addr().Network())
}

func NewServerInfo(name string, addr net.Addr) ServerInfo {
	return &serverInfo{name: name, addr: addr}
}

func (i *serverInfo) Name() string {
	return i.name
}

func (i *serverInfo) Addr() net.Addr {
	return i.addr
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

type registeredServer struct {
	info    ServerInfo
	players *players
}

func newRegisteredServer(info ServerInfo) *registeredServer {
	return &registeredServer{info: info, players: newPlayers()}
}

func (r *registeredServer) Equals(o RegisteredServer) bool {
	return r == nil && o == nil || (r != nil && o != nil &&
		r.ServerInfo().Equals(o.ServerInfo()))
}

func (r *registeredServer) ServerInfo() ServerInfo {
	return r.info
}

func (r *registeredServer) Players() Players {
	return r.players
}

var _ RegisteredServer = (*registeredServer)(nil)

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

	completedJoin      atomic.Bool
	gracefulDisconnect atomic.Bool
	lastPingId         atomic.Int64
	lastPingSent       atomic.Int64 // unix millis

	mu                      sync.RWMutex   // Protects following fields
	connection              *minecraftConn // the backend server connection
	connPhase               backendConnectionPhase
	activeDimensionRegistry *packet.DimensionRegistry
}

func newServerConnection(server *registeredServer, player *connectedPlayer) *serverConnection {
	return &serverConnection{server: server, player: player}
}

var _ ServerConnection = (*serverConnection)(nil)

// returns the backend server connection, nil-able
func (s *serverConnection) conn() *minecraftConn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connection
}

func (s *serverConnection) SendPluginMessage(id message.ChannelIdentifier, data []byte) error {
	panic("implement me") // TODO
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

// TODO support cancel connect via ctx
func (s *serverConnection) connect(ctx context.Context, resultFn internalConnectionResultFn) {
	addr := s.server.ServerInfo().Addr().String()

	// Connect proxy -> server
	zap.L().Debug("Proxy connecting to server to bridge player...", zap.String("addr", addr))
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		resultFn(nil, fmt.Errorf("error connecting to server %s: %w", addr, err))
		return
	}
	zap.L().Debug("Connected to server",
		zap.String("name", s.server.ServerInfo().Name()),
		zap.String("addr", addr))

	// Wrap server connection
	serverMc := newMinecraftConn(conn, s.player.proxy, false, func() []zap.Field {
		return []zap.Field{
			zap.Bool("isBackendServerConnection", true),
			zap.String("serverName", s.Server().ServerInfo().Name()),
			zap.Stringer("serverAddr", s.Server().ServerInfo().Addr()),
			zap.Stringer("forPlayer", s.player),
			zap.Stringer("forPlayerUuid", s.player.Id()),
		}
	})

	s.mu.Lock()
	s.connection = serverMc
	s.connection.setSessionHandler0(newBackendLoginSessionHandler(s, resultFn))
	s.connPhase = serverMc.connType.initialBackendPhase()
	s.mu.Unlock()

	zap.L().Debug("Started bridging backend server to player",
		zap.String("addr", addr),
		zap.String("server", s.server.ServerInfo().Name()))
	zap.String("player", s.player.Username())

	// Initiate the handshake.
	protocol := s.player.Protocol()
	handshake := &packet.Handshake{
		ProtocolVersion: int(protocol),
		NextStatus:      int(proto.LoginState),
	}
	host, port, err := net.SplitHostPort(s.server.ServerInfo().Addr().String())
	if err != nil { // should never happen, as we validated it already
		panic(err)
	}
	if s.config().Forwarding.Mode == config.LegacyForwardingMode {
		handshake.ServerAddress = s.createLegacyForwardingAddress()
	} else if s.player.Type() == LegacyForge {
		handshake.ServerAddress = fmt.Sprintf("%s%s", host, forge.HandshakeHostnameToken)
	} else {
		handshake.ServerAddress = host
	}
	p, _ := strconv.Atoi(port)
	handshake.Port = int16(p)

	if serverMc.BufferPacket(handshake) != nil {
		return
	}

	// Set server's protocol & state
	// after writing handshake, but before writing ServerLogin
	serverMc.SetProtocol(protocol)
	serverMc.SetState(state.Login)

	// Kick off the connection process
	// connection from proxy -> server (backend)
	if serverMc.WritePacket(&packet.ServerLogin{Username: s.player.Username()}) != nil {
		return
	}
	go serverMc.readLoop()
}

func (s *serverConnection) createLegacyForwardingAddress() string {
	// BungeeCord IP forwarding is simply a special injection after the "address" in the handshake,
	// separated by \0 (the null byte). In order, you send the original host, the player's IP, their
	// UUID (undashed), and if you are in online-mode, their login properties (from Mojang).
	b := new(strings.Builder)
	//host, _, _ := net.SplitHostPort(s.server.ServerInfo().Addr().String())
	b.WriteString(s.server.ServerInfo().Addr().String())
	b.WriteString("\000")
	b.WriteString(s.player.RemoteAddr().String())
	b.WriteString("\000")
	b.WriteString(s.player.GameProfile().Id.Undashed())
	b.WriteString("\000")
	props, err := json.Marshal(s.player.GameProfile().Properties)
	if err != nil { // should never happen
		panic(err)
	}
	b.WriteString(string(props)) // first convert props to string
	return b.String()
}

// Returns the active backend server connection or false if inactive.
func (s *serverConnection) ensureConnected() (backend *minecraftConn, connected bool) {
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
	if s.connection != nil {
		s.gracefulDisconnect.Store(true)
		if !s.connection.Closed() { // only close if not already closing to prevent deadlock
			_ = s.connection.closeKnown(false)
		}
		s.connection = nil // nil means not connected
	}
}

func (s *serverConnection) setActiveDimensionRegistry(registry *packet.DimensionRegistry) {
	s.mu.Lock()
	s.activeDimensionRegistry = registry
	s.mu.Unlock()
}

// Indicates that we have completed the plugin process.
func (s *serverConnection) completeJoin() {
	s.mu.Lock() // Yes, lock whole function including atomic completedJoin
	if s.completedJoin.CAS(false, true) {
		if s.connPhase == unknownBackendPhase {
			// Now we know
			s.connPhase = vanillaBackendPhase
			if s.connection != nil {
				s.connection.connType = vanillaConnectionType
			}
		}
	}
	s.mu.Unlock()
}

func (s *serverConnection) config() *config.Config {
	return s.player.proxy.Config()
}

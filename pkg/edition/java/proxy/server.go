package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.minekube.com/gate/pkg/edition/java/forge/modernforge"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"

	"github.com/dboslee/lru"
	"github.com/go-logr/logr"
	"go.uber.org/atomic"

	"go.minekube.com/gate/pkg/edition/java/internal/protoutil"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/phase"
	"go.minekube.com/gate/pkg/gate/proto"

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

// PlayersToSlice returns a slice of all players.
func PlayersToSlice[R any](p Players) []R {
	var s []R
	p.Range(func(p Player) bool {
		r, ok := p.(R)
		if ok {
			s = append(s, r)
		}
		return true
	})
	return s
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
	Players() Players // The players connected to the server on THIS proxy.
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

func (r *registeredServer) ServerInfo() ServerInfo {
	return r.info
}

func (r *registeredServer) Players() Players {
	return r.players
}

var _ RegisteredServer = (*registeredServer)(nil)

// BroadcastPluginMessage sends the plugin message to all players on the server.
func BroadcastPluginMessage(sinks []message.ChannelMessageSink, identifier message.ChannelIdentifier, data []byte) {
	for _, sink := range sinks {
		go func(s message.ChannelMessageSink) { _ = s.SendPluginMessage(identifier, data) }(sink)
	}
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

	previousServer *registeredServer // nil-able

	completedJoin      atomic.Bool
	gracefulDisconnect atomic.Bool
	pendingPings       *lru.SyncCache[int64, time.Time]

	entityID int // entity ID of the player on this server connection

	mu         sync.RWMutex        // Protects following fields
	connection netmc.MinecraftConn // the backend server connection
	connPhase  phase.BackendConnectionPhase
}

func newServerConnection(server *registeredServer, previousServer *registeredServer, player *connectedPlayer) *serverConnection {
	return &serverConnection{
		server:         server,
		player:         player,
		previousServer: previousServer,
		pendingPings:   lru.NewSync[int64, time.Time](lru.WithCapacity(5)),
		log: player.log.WithName("serverConn").WithValues(
			"serverName", server.info.Name(),
			"serverAddr", server.info.Addr()),
	}
}

var _ ServerConnection = (*serverConnection)(nil)

// returns the backend server connection, nil-able
func (s *serverConnection) conn() netmc.MinecraftConn {
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
		return netmc.ErrClosedConn
	}
	return mc.WritePacket(&plugin.Message{
		Channel: id.ID(),
		Data:    data,
	})
}

func (s *serverConnection) Server() RegisteredServer {
	if s == nil {
		return nil
	}
	return s.server
}

func (s *serverConnection) Player() Player {
	if s == nil {
		return nil
	}
	return s.player
}

func (s *serverConnection) SetPhase(phase phase.BackendConnectionPhase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connPhase = phase
}
func (s *serverConnection) phase() phase.BackendConnectionPhase {
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
	ctx, span := tracer.Start(ctx, "serverConnection.dial", trace.WithAttributes(
		attribute.String("server.name", s.server.info.Name()),
		attribute.Stringer("server.addr", s.server.info.Addr()),
	))
	defer span.End()

	var (
		sd ServerDialer
		ok bool
	)
	if sd, ok = s.Server().ServerInfo().(ServerDialer); !ok {
		if sd, ok = s.Server().(ServerDialer); !ok {
			dstAddr := s.Server().ServerInfo().Addr()
			span.SetAttributes(attribute.Stringer("server.addr", dstAddr))
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", dstAddr.String())
			if err != nil {
				return nil, err
			}

			return conn, nil
		}
	}
	return sd.Dial(ctx, s.player)
}

// HandshakeAddresser provides the ServerAddress sent with the packet.Handshake when a player joins the server
// implementing this interface.
// A ServerInfo of a registered server or a RegisteredServer can implement this interface.
// If no ServerInfo or RegisteredServer implements this interface the ServerInfo.Addr the default ServerAddress is used
// or the BungeeCord forwarding scheme if the proxy is in cfg.LegacyForwardingMode.
type HandshakeAddresser interface {
	HandshakeAddr(defaultPlayerVirtualHost string, player Player) (newPlayerVirtualHost string)
}

func (s *serverConnection) handshakeAddr(vHost string, player Player) string {
	var ha HandshakeAddresser
	var ok bool
	if ha, ok = s.Server().ServerInfo().(HandshakeAddresser); !ok {
		if ha, ok = s.Server().(HandshakeAddresser); !ok {
			switch s.config().Forwarding.Mode {
			case config.LegacyForwardingMode:
				return s.createLegacyForwardingAddress()
			case config.BungeeGuardForwardingMode:
				secret := s.config().Forwarding.BungeeGuardSecret
				return s.createBungeeGuardForwardingAddress(secret)
			}
		}
	}
	if ha != nil {
		vHost = ha.HandshakeAddr(vHost, player)
	}
	if s.player.Type() == phase.LegacyForge {
		vHost += forge.HandshakeHostnameToken
	} else if s.player.Type() == phase.ModernForge {
		vHost = modernforge.ModernToken(vHost)
	}
	return vHost
}

func (s *serverConnection) connect(ctx context.Context) (result *connectionResult, err error) {
	// Connect proxy -> server
	debug := s.log.V(1)
	debug.Info("dialing backend server...")
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to backend server %q: %w", s.server.ServerInfo().Name(), err)
	}

	if s.config().ProxyProtocolBackend {
		header := protoutil.ProxyHeader(s.player.RemoteAddr(), conn.RemoteAddr())
		debug.Info("writing proxy protocol header")
		if _, err = header.WriteTo(conn); err != nil {
			return nil, fmt.Errorf("error writing proxy protocol header to backend: %w", err)
		}
	}

	debug.Info("connected to server")

	// Wrap server connection
	logCtx := logr.NewContext(
		context.Background(),
		logr.FromContextOrDiscard(s.player.MinecraftConn.Context()),
	)
	serverMc, readLoop := netmc.NewMinecraftConn(
		logCtx, conn, proto.ClientBound,
		time.Duration(s.config().ReadTimeout)*time.Millisecond,
		time.Duration(s.config().ConnectionTimeout)*time.Millisecond,
		s.config().Compression.Level,
	)
	resultChan := make(chan *connResponse, 1)

	// Kick off the connection process...
	if !serverMc.SwitchSessionHandler(state.Handshake) {
		handler := newBackendLoginSessionHandler(s, &connRequestCxt{
			Context:  ctx,
			response: resultChan,
		}, s.player.sessionHandlerDeps)
		serverMc.SetActiveSessionHandler(state.Handshake, handler)
		serverMc.AddSessionHandler(state.Login, handler)
	}

	// Set the connection phase, which may, for future forge (or whatever), be
	// determined at this point already
	s.mu.Lock()
	s.connection = serverMc
	s.connPhase = serverMc.Type().InitialBackendPhase()
	s.mu.Unlock()

	debug.Info("establishing player connection with server...")
	return s.startHandshake(readLoop, resultChan)
}

func (s *serverConnection) startHandshake(
	readLoop func(),
	resultChan chan *connResponse,
) (*connectionResult, error) {
	serverMc, ok := s.ensureConnected()
	if !ok {
		return nil, netmc.ErrClosedConn
	}

	// Initiate the handshake.
	protocol := s.player.Protocol()
	handshake := &packet.Handshake{
		ProtocolVersion: int(protocol),
		NextStatus:      int(states.LoginState),
		Port:            int(netutil.Port(s.server.ServerInfo().Addr())),
	}

	// Set handshake ServerAddress
	{
		playerVHost := netutil.Host(s.player.virtualHost)
		if playerVHost == "" {
			playerVHost = netutil.Host(s.server.ServerInfo().Addr())
		}
		handshake.ServerAddress = s.handshakeAddr(playerVHost, s.player)
	}
	if err := serverMc.BufferPacket(handshake); err != nil {
		return nil, fmt.Errorf("error buffer handshake packet in server connection: %w", err)
	}

	// Set server's protocol & state
	// after writing handshake, but before writing ServerLogin
	serverMc.SetProtocol(protocol)
	if !serverMc.SwitchSessionHandler(state.Login) {
		return nil, fmt.Errorf("error switching session handler to login in server connection")
	}

	serverLogin := &packet.ServerLogin{
		Username: s.player.Username(),
	}
	if s.player.IdentifiedKey() == nil &&
		s.player.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
		serverLogin.HolderID = s.player.ID()
	} else {
		serverLogin.PlayerKey = s.player.IdentifiedKey()
	}

	// Kick off the connection process
	// connection from proxy -> backend server
	if err := serverMc.WritePacket(serverLogin); err != nil {
		return nil, fmt.Errorf("error writing ServerLogin packet to server connection: %w", err)
	}
	go readLoop()

	// Block until we get a result
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
	const sep = "\000"
	b.WriteString(sep)
	b.WriteString(playerIP)
	b.WriteString(sep)
	b.WriteString(s.player.profile.ID.Undashed())
	b.WriteString(sep)
	props, err := json.Marshal(s.player.profile.Properties)
	if err != nil { // should never happen
		panic(err)
	}
	b.WriteString(string(props)) // first convert props to string
	return b.String()
}

func (s *serverConnection) createBungeeGuardForwardingAddress(secret string) string {
	// Bungeeguard IP forwading is the same as the legacy Bungeecord IP forwarding but with an additional
	// property in the profile properties that contains the bungeeguard-token.
	playerIP := netutil.Host(s.player.RemoteAddr())
	b := new(strings.Builder)
	b.WriteString(s.server.ServerInfo().Addr().String())
	const sep = "\000"
	b.WriteString(sep)
	b.WriteString(playerIP)
	b.WriteString(sep)
	b.WriteString(s.player.profile.ID.Undashed())
	b.WriteString(sep)
	props, err := json.Marshal(
		append(s.player.profile.Properties, profile.Property{Name: "bungeeguard-token", Value: secret}))
	if err != nil { // should never happen
		panic(err)
	}
	b.WriteString(string(props)) // first convert props to string
	return b.String()
}

// Returns the active backend server connection or false if inactive.
func (s *serverConnection) ensureConnected() (backend netmc.MinecraftConn, connected bool) {
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
	return conn != nil && !netmc.Closed(conn) &&
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
		if !netmc.Closed(s.connection) { // only close if not already closing to prevent deadlock
			_ = netmc.CloseUnknown(s.connection)
		}
		s.connection = nil // nil means not connected
	}
}

// Indicates that we have completed the plugin process.
func (s *serverConnection) completeJoin() {
	if s.completedJoin.CompareAndSwap(false, true) {
		s.mu.Lock()
		if s.connPhase == phase.UnknownBackendPhase {
			// Now we know
			s.connPhase = phase.VanillaBackendPhase
			if s.connection != nil {
				s.connection.SetType(phase.Vanilla)
			}
		}
		s.mu.Unlock()
	}
}

func (s *serverConnection) config() *config.Config {
	return s.player.config()
}

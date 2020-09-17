package proxy

import (
	"context"
	"errors"
	"fmt"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"go.minekube.com/gate/internal/health"
	"go.minekube.com/gate/internal/util/auth"
	"go.minekube.com/gate/internal/util/quotautil"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/edition/java/proto"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/util"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/sets"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	rpc "google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"strings"
	"sync"
	"time"
)

// Proxy is Gate's Java edition of a Minecraft proxy.
type Proxy struct {
	config           *config.Config
	event            *event.Manager
	command          *CommandManager
	channelRegistrar *ChannelRegistrar
	authenticator    *auth.Authenticator

	startTime time.Time
	runOnce   atomic.Bool
	closeOnce sync.Once
	closed    chan struct{}

	shutdownReason *component.Text
	motd           *component.Text
	favicon        favicon.Favicon

	muS     sync.RWMutex                 // Protects following field
	servers map[string]*registeredServer // registered backend servers: by lower case names

	muP         sync.RWMutex                   // Protects following fields
	playerNames map[string]*connectedPlayer    // lower case usernames map
	playerIDs   map[uuid.UUID]*connectedPlayer // uuids map

	connectionsQuota *quotautil.Quota
	loginsQuota      *quotautil.Quota
}

// New takes a config that should have been validated by
// config.Validate and returns a new initialized Proxy ready to run.
func New(config config.Config) (p *Proxy) {
	p = &Proxy{
		closed:           make(chan struct{}),
		config:           &config,
		event:            event.NewManager(),
		command:          newCommandManager(),
		channelRegistrar: NewChannelRegistrar(),
		servers:          map[string]*registeredServer{},
		playerNames:      map[string]*connectedPlayer{},
		playerIDs:        map[uuid.UUID]*connectedPlayer{},
		authenticator:    auth.NewAuthenticator(),
	}
	// Connection & login rate limiters
	quota := config.Quota.Connections
	if quota.Enabled {
		p.connectionsQuota = quotautil.NewQuota(quota.OPS, quota.Burst, quota.MaxEntries)
	}
	quota = config.Quota.Logins
	if quota.Enabled {
		p.loginsQuota = quotautil.NewQuota(quota.OPS, quota.Burst, quota.MaxEntries)
	}
	return p
}

// Returned by Proxy.Run if the proxy instance was already run.
var ErrProxyAlreadyRun = errors.New("proxy was already run, create a new one")

// Start runs the Java edition Proxy, blocks until the proxy is
// Shutdown or an error occurred while starting.
// The Proxy is already shutdown on method return.
// In order to stop the Proxy call Shutdown.
// A Proxy can only be run once or ErrProxyAlreadyRun is returned.
func (p *Proxy) Start(stop <-chan struct{}) error {
	if !p.runOnce.CAS(false, true) {
		return ErrProxyAlreadyRun
	}
	return p.run(stop) // Run and block
}

// Shutdown stops the Proxy and/or blocks until the Proxy has finished shutdown.
//
// It first stops listening for new connections, disconnects
// all existing connections with the given reason (nil = blank reason)
// and waits for all event subscribers to finish.
func (p *Proxy) Shutdown(reason component.Component) {
	p.closeOnce.Do(func() {
		zap.L().Info("Shutting down the proxy...")
		defer zap.L().Info("Finished shutdown.")

		pre := &PreShutdownEvent{reason: reason}
		p.event.Fire(pre)
		reason = pre.Reason()

		zap.L().Info("Disconnecting all players...")
		close(p.closed)
		p.DisconnectAll(reason)

		zap.L().Info("Waiting for all event handlers to complete...")
		p.event.Fire(&ShutdownEvent{})
		p.event.Wait()
	})
}

// called before starting to actually run the proxy
func (p *Proxy) preInit() (err error) {
	// Load shutdown reason
	if err = p.loadShutdownReason(); err != nil {
		return fmt.Errorf("error loading shutdown reason: %v", err)
	}
	// Load status motd
	if err = p.loadMotd(); err != nil {
		return fmt.Errorf("error loading status motd: %v", err)
	}
	// Load favicon
	if err = p.loadFavicon(); err != nil {
		return fmt.Errorf("error loading favicon: %v", err)
	}

	c := p.config
	// Register servers
	for name, addr := range c.Servers {
		p.Register(NewServerInfo(name, tcpAddr(addr)))
	}
	if len(c.Servers) != 0 {
		zap.S().Infof("Pre-registered %d servers", len(c.Servers))
	}

	// Register builtin commands
	if c.BuiltinCommands {
		p.registerBuiltinCommands()
	}

	// Init "plugins" with the proxy
	return p.initPlugins()
}

// loads shutdown kick reason on proxy shutdown from the config
func (p *Proxy) loadShutdownReason() (err error) {
	c := p.config
	if len(c.ShutdownReason) == 0 {
		return nil
	}
	p.motd, err = parseTextComponentFromConfig(c.ShutdownReason)
	return
}

func (p *Proxy) loadMotd() (err error) {
	c := p.config
	if len(c.Status.Motd) == 0 {
		return nil
	}
	p.motd, err = parseTextComponentFromConfig(c.Status.Motd)
	return
}

func parseTextComponentFromConfig(s string) (t *component.Text, err error) {
	var c component.Component
	if strings.HasPrefix(s, "{") {
		c, err = util.LatestJsonCodec().Unmarshal([]byte(s))
	} else {
		c, err = (&legacy.Legacy{}).Unmarshal([]byte(s))
	}
	if err != nil {
		return nil, err
	}
	t, ok := c.(*component.Text)
	if !ok {
		return nil, errors.New("invalid text component")
	}
	return t, nil
}

// initializes favicon from the config
func (p *Proxy) loadFavicon() (err error) {
	c := p.config
	if len(c.Status.Favicon) == 0 {
		return nil
	}
	if strings.HasPrefix(c.Status.Favicon, "data:image/") {
		p.favicon = favicon.Favicon(c.Status.Favicon)
		zap.L().Info("Using favicon from data uri")
	} else {
		p.favicon, err = favicon.FromFile(c.Status.Favicon)
		if err != nil {
			return fmt.Errorf("error reading favicon file %q: %w", c.Status.Favicon, err)
		}
		zap.S().Infof("Using favicon file %s", c.Status.Favicon)
	}
	return nil
}

func (p *Proxy) initPlugins() error {
	for _, pl := range Plugins {
		if err := pl.Init(p); err != nil {
			return fmt.Errorf("error running init hook for plugin %q: %w", pl.Name, err)
		}
	}
	return nil
}

func (p *Proxy) run(stop <-chan struct{}) error {
	p.startTime = time.Now().UTC()
	if err := p.preInit(); err != nil {
		return fmt.Errorf("pre-initialization error: %w", err)
	}
	go func() { <-stop; p.Shutdown(p.shutdownReason) }()

	errChan := make(chan error, 1)
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	if p.config.Health.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errChan <- p.runHealthService(p.closed)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		errChan <- p.listenAndServe(p.config.Bind, p.closed)
	}()

	return <-errChan
}

func (p *Proxy) runHealthService(stop <-chan struct{}) error {
	probe := p.config.Health
	run, err := health.New(probe.Bind)
	if err != nil {
		return fmt.Errorf("error creating health probe service: %w", err)
	}
	zap.S().Infof("Health probe service running at %s", probe.Bind)
	return run(stop, p.healthCheck)
}

// Event returns the Proxy's event manager.
func (p *Proxy) Event() *event.Manager {
	return p.event
}

// Command returns the Proxy's command manager.
func (p *Proxy) Command() *CommandManager {
	return p.command
}

// Config returns the config used by the Proxy.
func (p *Proxy) Config() config.Config {
	return *p.config
}

// Server gets a backend server registered with the proxy by name.
// Returns nil if not found.
func (p *Proxy) Server(name string) RegisteredServer {
	return p.server(name)
}

func (p *Proxy) server(name string) *registeredServer {
	name = strings.ToLower(name)
	p.muS.RLock()
	defer p.muS.RUnlock()
	return p.servers[name] // may be nil
}

// Servers gets all registered servers.
func (p *Proxy) Servers() []RegisteredServer {
	p.muS.RLock()
	defer p.muS.RUnlock()
	l := make([]RegisteredServer, 0, len(p.servers))
	for _, rs := range p.servers {
		l = append(l, rs)
	}
	return l
}

// Register registers a server with the proxy.
//
// Returns the new registered server and true on success.
//
// On failure either:
//  - if name already exists, returns the already registered server and false
//  - if the specified ServerInfo is invalid, returns nil and false.
func (p *Proxy) Register(info ServerInfo) (RegisteredServer, bool) {
	if info == nil || !config.ValidServerName(info.Name()) ||
		config.ValidHostPort(info.Addr().String()) != nil {
		return nil, false
	}

	name := strings.ToLower(info.Name())

	p.muS.Lock()
	defer p.muS.Unlock()
	if exists, ok := p.servers[name]; ok {
		return exists, false
	}
	rs := newRegisteredServer(info)
	p.servers[name] = rs

	zap.S().Debugf("Registered server %q (%s)", info.Name(), info.Addr())
	return rs, true
}

// Unregister unregisters the server exactly matching the
// given ServerInfo and returns true if found.
func (p *Proxy) Unregister(info ServerInfo) bool {
	if info == nil {
		return false
	}
	name := strings.ToLower(info.Name())
	p.muS.Lock()
	defer p.muS.Unlock()
	rs, ok := p.servers[name]
	if !ok || !rs.ServerInfo().Equals(info) {
		return false
	}
	delete(p.servers, name)

	zap.S().Debugf("Unregistered server %q (%s)", info.Name(), info.Addr())
	return true
}

// DisconnectAll disconnects all current connected players in parallel.
// It is done in parallel
func (p *Proxy) DisconnectAll(reason component.Component) {
	p.muP.RLock()
	players := p.playerIDs
	p.muP.RUnlock()

	var wg sync.WaitGroup
	wg.Add(len(players))
	for _, p := range players {
		go func(p *connectedPlayer) {
			p.Disconnect(reason)
			wg.Done()
		}(p)
	}
	wg.Wait()
}

// listenAndServe starts listening for connections on addr until closed channel receives.
func (p *Proxy) listenAndServe(addr string, stop <-chan struct{}) error {
	select {
	case <-stop:
		return nil
	default:
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	go func() {
		<-stop
		_ = ln.Close()
	}()

	p.event.Fire(&ReadyEvent{})

	zap.S().Infof("Listening on %s", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			var opErr *net.OpError
			if errors.As(err, &opErr) && errs.IsConnClosedErr(opErr.Err) {
				// Listener was closed
				return nil
			}
			return fmt.Errorf("error accepting new connection: %w", err)
		}
		go p.handleRawConn(conn)
	}
}

// handleRawConn handles a just-accepted connection that
// has not had any I/O performed on it yet.
func (p *Proxy) handleRawConn(raw net.Conn) {
	if p.connectionsQuota != nil && p.connectionsQuota.Blocked(raw.RemoteAddr()) {
		_ = raw.Close()
		zap.L().Info("A connection was exceeded the rate limit", zap.Stringer("remoteAddr", raw.RemoteAddr()))
		return
	}

	// Create client connection
	conn := newMinecraftConn(raw, p, true, func() []zap.Field {
		return []zap.Field{zap.Bool("player", true)}
	})
	conn.setSessionHandler0(newHandshakeSessionHandler(conn))
	// Read packets in loop
	conn.readLoop()
}

// PlayerCount returns the number of players on the proxy.
func (p *Proxy) PlayerCount() int {
	p.muP.RLock()
	defer p.muP.RUnlock()
	return len(p.playerIDs)
}

// Players returns all players on the proxy.
func (p *Proxy) Players() []Player {
	p.muP.RLock()
	defer p.muP.RUnlock()
	pls := make([]Player, 0, len(p.playerIDs))
	for _, player := range p.playerIDs {
		pls = append(pls, player)
	}
	return pls
}

// Player returns the online player by their Minecraft id.
// Returns nil if the player was not found.
func (p *Proxy) Player(id uuid.UUID) Player {
	p.muP.RLock()
	defer p.muP.RUnlock()
	return p.playerIDs[id]
}

// Player returns the online player by their Minecraft name (search is case-insensitive).
// Returns nil if the player was not found.
func (p *Proxy) PlayerByName(username string) Player {
	return p.playerByName(username)
}
func (p *Proxy) playerByName(username string) *connectedPlayer {
	p.muP.RLock()
	defer p.muP.RUnlock()
	return p.playerNames[strings.ToLower(username)]
}

func (p *Proxy) canRegisterConnection(player *connectedPlayer) bool {
	c := p.config
	if c.OnlineMode && c.OnlineModeKickExistingPlayers {
		return true
	}
	lowerName := strings.ToLower(player.Username())
	p.muP.RLock()
	defer p.muP.RUnlock()
	return p.playerNames[lowerName] == nil && p.playerIDs[player.ID()] == nil
}

// Attempts to register the connection with the proxy.
func (p *Proxy) registerConnection(player *connectedPlayer) bool {
	lowerName := strings.ToLower(player.Username())
	c := p.config

retry:
	p.muP.Lock()
	if c.OnlineModeKickExistingPlayers {
		existing, ok := p.playerIDs[player.ID()]
		if ok {
			// Make sure we disconnect existing duplicate
			// player connection before we register the new one.
			//
			// Disconnecting the existing connection will call p.unregisterConnection in the
			// teardown needing the p.muP.Lock() so we unlock.
			p.muP.Unlock()
			existing.disconnectDueToDuplicateConnection.Store(true)
			existing.Disconnect(&component.Translation{
				Key: "multiplayer.disconnect.duplicate_login",
			})
			// Now we can retry in case another duplicate connection
			// occurred before we could acquire the lock at `retry`.
			//
			// Meaning we keep disconnecting incoming duplicates until
			// we can register our connection, but this shall be uncommon anyways. :)
			goto retry
		}
	} else {
		_, exists := p.playerNames[lowerName]
		if exists {
			return false
		}
		_, exists = p.playerIDs[player.ID()]
		if exists {
			return false
		}
	}

	p.playerIDs[player.ID()] = player
	p.playerNames[lowerName] = player
	p.muP.Unlock()
	return true
}

// unregisters a connected player
func (p *Proxy) unregisterConnection(player *connectedPlayer) (found bool) {
	p.muP.Lock()
	defer p.muP.Unlock()
	_, found = p.playerIDs[player.ID()]
	delete(p.playerNames, strings.ToLower(player.Username()))
	delete(p.playerIDs, player.ID())
	// TODO p.s.bossBarManager.onDisconnect(player)?
	return found
}

//
//
//
//
//
//

func (p *Proxy) ChannelRegistrar() *ChannelRegistrar {
	return p.channelRegistrar
}

type ChannelRegistrar struct {
	mu          sync.RWMutex // Protects following fields
	identifiers map[string]message.ChannelIdentifier
}

func NewChannelRegistrar() *ChannelRegistrar {
	return &ChannelRegistrar{identifiers: map[string]message.ChannelIdentifier{}}
}

// ChannelsForProtocol returns all the channel names
// to register depending on the Minecraft protocol version.
func (r *ChannelRegistrar) ChannelsForProtocol(protocol proto.Protocol) sets.String {
	if protocol.GreaterEqual(proto.Minecraft_1_13) {
		return r.ModernChannelIDs()
	}
	return r.LegacyChannelIDs()
}

// ModernChannelIDs returns all channel IDs (as strings)
// for use with Minecraft 1.13 and above.
func (r *ChannelRegistrar) ModernChannelIDs() sets.String {
	r.mu.RLock()
	ids := r.identifiers
	r.mu.RUnlock()
	ss := sets.String{}
	for _, i := range ids {
		if _, ok := i.(*message.MinecraftChannelIdentifier); ok {
			ss.Insert(i.ID())
		} else {
			ss.Insert(plugin.TransformLegacyToModernChannel(i.ID()))
		}
	}
	return ss
}

// LegacyChannelIDs returns all legacy channel IDs.
func (r *ChannelRegistrar) LegacyChannelIDs() sets.String {
	r.mu.RLock()
	ids := r.identifiers
	r.mu.RUnlock()
	ss := sets.String{}
	for _, i := range ids {
		ss.Insert(i.ID())
	}
	return ss
}

func (r *ChannelRegistrar) FromID(channel string) (message.ChannelIdentifier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.identifiers[channel]
	return id, ok
}

//
//
//
//
//

// pings the proxy to check health
func (p *Proxy) healthCheck(c context.Context) (*rpc.HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(c, time.Second)
	defer cancel()

	var dialer net.Dialer
	client, err := dialer.DialContext(ctx, "tcp", p.config.Bind)
	if err != nil {
		return &rpc.HealthCheckResponse{Status: rpc.HealthCheckResponse_NOT_SERVING}, nil
	}
	defer client.Close()

	return &rpc.HealthCheckResponse{Status: rpc.HealthCheckResponse_SERVING}, nil
}

// sends msg to all players on this proxy
func (p *Proxy) sendMessage(msg component.Component) {
	p.muP.RLock()
	players := p.playerIDs
	p.muP.RUnlock()
	for _, p := range players {
		go func(p Player) { _ = p.SendMessage(msg) }(p)
	}
}

//
//
//

func withConnectionTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(cfg.ConnectionTimeout)*time.Millisecond)
}

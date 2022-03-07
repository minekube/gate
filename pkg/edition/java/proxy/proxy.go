package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/plugin"
	util2 "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/addrquota"
	"go.minekube.com/gate/pkg/runtime/event"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/favicon"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/sets"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.minekube.com/gate/pkg/util/validation"
)

// Proxy is Gate's Java edition Minecraft proxy.
type Proxy struct {
	log              logr.Logger
	config           *config.Config
	event            event.Manager
	command          command.Manager
	channelRegistrar *ChannelRegistrar
	authenticator    auth.Authenticator

	startTime atomic.Value

	closeMu       sync.Mutex
	closeListener chan struct{}
	started       bool

	shutdownReason *component.Text
	motd           *component.Text
	favicon        favicon.Favicon

	muS     sync.RWMutex                 // Protects following field
	servers map[string]*registeredServer // registered backend servers: by lower case names

	muP         sync.RWMutex                   // Protects following fields
	playerNames map[string]*connectedPlayer    // lower case usernames map
	playerIDs   map[uuid.UUID]*connectedPlayer // uuids map

	connectionsQuota *addrquota.Quota
	loginsQuota      *addrquota.Quota
}

// Options are the options for a new Java edition Proxy.
type Options struct {
	// Config requires configuration
	// validated by config.Validate.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
	// Logger is the logger to be used by the Proxy.
	// If none is set, does no logging at all.
	Logger logr.Logger
	// Authenticator to authenticate users in online mode.
	// If not set, creates a default one.
	Authenticator auth.Authenticator
}

// New returns a new Proxy ready to start.
func New(options Options) (p *Proxy, err error) {
	if options.Config == nil {
		return nil, errs.ErrMissingConfig
	}
	log := options.Logger
	if log == nil {
		log = logr.NopLog
	}
	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}
	authn := options.Authenticator
	if authn == nil {
		authn, err = auth.New(auth.Options{})
		if err != nil {
			return nil, fmt.Errorf("erorr creating authenticator: %w", err)
		}
	}

	p = &Proxy{
		log:              log,
		config:           options.Config,
		event:            eventMgr,
		channelRegistrar: NewChannelRegistrar(),
		servers:          map[string]*registeredServer{},
		playerNames:      map[string]*connectedPlayer{},
		playerIDs:        map[uuid.UUID]*connectedPlayer{},
		authenticator:    authn,
	}

	c := options.Config
	// Connection & login rate limiters
	quota := c.Quota.Connections
	if quota.Enabled {
		p.connectionsQuota = addrquota.NewQuota(quota.OPS, quota.Burst, quota.MaxEntries)
	}
	quota = c.Quota.Logins
	if quota.Enabled {
		p.loginsQuota = addrquota.NewQuota(quota.OPS, quota.Burst, quota.MaxEntries)
	}

	return p, nil
}

// ErrProxyAlreadyRun is returned by Proxy.Run if the proxy instance was already run.
var ErrProxyAlreadyRun = errors.New("proxy was already run, create a new one")

// Start runs the Java edition Proxy, blocks until the proxy is
// Shutdown or an error occurred while starting.
// The Proxy is already shutdown on method return.
// Another method of stopping the Proxy is to call Shutdown.
// A Proxy can only be run once or ErrProxyAlreadyRun is returned.
func (p *Proxy) Start(stop <-chan struct{}) error {
	p.closeMu.Lock()
	if p.started {
		p.closeMu.Unlock()
		return ErrProxyAlreadyRun
	}
	p.started = true
	p.startTime.Store(time.Now().UTC())

	stopListener := make(chan struct{})
	go func() {
		<-stop
		select {
		case <-stopListener:
		default:
			close(stopListener)
		}
	}()
	p.closeListener = stopListener
	p.closeMu.Unlock()

	if err := p.preInit(); err != nil {
		return fmt.Errorf("pre-initialization error: %w", err)
	}
	defer p.Shutdown(p.shutdownReason) // disconnects players
	return p.listenAndServe(p.config.Bind, stopListener)
}

// Shutdown stops the Proxy and/or blocks until the Proxy has finished shutdown.
//
// It first stops listening for new connections, disconnects
// all existing connections with the given reason (nil = blank reason)
// and waits for all event subscribers to finish.
func (p *Proxy) Shutdown(reason component.Component) {
	p.closeMu.Lock()
	defer p.closeMu.Unlock()
	if !p.started {
		return // not started or already shutdown
	}
	p.started = false
	select {
	case <-p.closeListener: // channel may be already closed
	default:
		// stop listening for new connections
		close(p.closeListener)
	}

	p.log.Info("Shutting down the proxy...")
	shutdownTime := time.Now()
	defer func() {
		p.log.Info("Finished shutdown.",
			"shutdownTime", time.Since(shutdownTime).Round(time.Microsecond).String(),
			"totalTime", time.Since(p.startTime.Load().(time.Time)).Round(time.Millisecond).String())
	}()

	pre := &PreShutdownEvent{reason: reason}
	p.event.Fire(pre)
	reason = pre.Reason()

	reasonStr := new(strings.Builder)
	if reason != nil {
		err := (&legacy.Legacy{}).Marshal(reasonStr, reason)
		if err != nil {
			p.log.Error(err, "Error marshal disconnect reason to legacy format")
		}
	}

	p.log.Info("Disconnecting all players...", "reason", reasonStr.String())
	disconnectTime := time.Now()
	p.DisconnectAll(reason)
	p.log.Info("Disconnected all players.", "time", time.Since(disconnectTime).String())

	p.log.Info("Waiting for all event handlers to complete...")
	p.event.Fire(&ShutdownEvent{})
	p.event.Wait()
}

// called before starting to actually run the proxy
func (p *Proxy) preInit() (err error) {
	// Load shutdown reason
	if err = p.loadShutdownReason(); err != nil {
		return fmt.Errorf("error loading shutdown reason: %w", err)
	}
	// Load status motd
	if err = p.loadMotd(); err != nil {
		return fmt.Errorf("error loading status motd: %w", err)
	}
	// Load favicon
	if err = p.loadFavicon(); err != nil {
		return fmt.Errorf("error loading favicon: %w", err)
	}

	c := p.config
	// Register servers
	if len(c.Servers) != 0 {
		p.log.Info("Registering servers...", "count", len(c.Servers))
	}
	for name, addr := range c.Servers {
		pAddr, err := netutil.Parse(addr, "tcp")
		if err != nil {
			return fmt.Errorf("error parsing server %q address %q: %w", name, addr, err)
		}
		info := NewServerInfo(name, pAddr)
		_, err = p.Register(info)
		if err != nil {
			p.log.Info("Could not register server", "server", info)
		}
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
	p.shutdownReason, err = parseTextComponentFromConfig(c.ShutdownReason)
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
		c, err = util2.LatestJsonCodec().Unmarshal([]byte(s))
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
		p.log.Info("Using favicon from data uri", "length", len(p.favicon))
	} else {
		p.favicon, err = favicon.FromFile(c.Status.Favicon)
		if err != nil {
			return fmt.Errorf("error reading favicon file %q: %w", c.Status.Favicon, err)
		}
		p.log.Info("Using favicon file", "file", c.Status.Favicon)
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

// Event returns the Proxy's event manager.
func (p *Proxy) Event() event.Manager {
	return p.event
}

// Command returns the Proxy's command manager.
func (p *Proxy) Command() *command.Manager {
	return &p.command
}

// Config returns the config used by the Proxy.
func (p *Proxy) Config() config.Config {
	return *p.config
}

// Server gets a backend server registered with the proxy by name.
// Returns nil if not found.
func (p *Proxy) Server(name string) RegisteredServer {
	s := p.server(name)
	if s == (*registeredServer)(nil) {
		return nil // return correct nil
	}
	return s
}

func (p *Proxy) server(name string) *registeredServer {
	name = strings.ToLower(name)
	p.muS.RLock()
	s := p.servers[name] // may be nil
	p.muS.RUnlock()
	return s
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

// ServerRegistry is used to retrieve registered servers that players can connect to.
type ServerRegistry interface {
	// Server gets a registered server by name or returns nil if not found.
	Server(name string) RegisteredServer
	ServerRegistrar
}

// ServerRegistrar is used to register servers.
type ServerRegistrar interface {
	// Register registers a server with the proxy and returns it.
	// If the there is already a server with the same info
	// error ErrServerAlreadyExists is returned and the already registered server.
	Register(info ServerInfo) (RegisteredServer, error)
	// Unregister unregisters the server exactly matching the
	// given ServerInfo and returns true if found.
	Unregister(info ServerInfo) bool
}

// ErrServerAlreadyExists indicates that a server is already registered in ServerRegistrar.
var ErrServerAlreadyExists = errors.New("server already exists")

var _ ServerRegistry = (*Proxy)(nil)

// Register - See ServerRegistrar
func (p *Proxy) Register(info ServerInfo) (RegisteredServer, error) {
	if info == nil {
		return nil, errors.New("info must not be nil")
	}
	if !validation.ValidServerName(info.Name()) {
		return nil, errors.New("invalid server name")
	}
	if err := validation.ValidHostPort(info.Addr().String()); err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	name := strings.ToLower(info.Name())

	p.muS.Lock()
	defer p.muS.Unlock()
	if exists, ok := p.servers[name]; ok {
		return exists, ErrServerAlreadyExists
	}
	rs := newRegisteredServer(info)
	p.servers[name] = rs

	p.log.Info("Registered new server", "name", info.Name(), "addr", info.Addr())
	return rs, nil
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
	if !ok || !ServerInfoEqual(rs.ServerInfo(), info) {
		return false
	}
	delete(p.servers, name)

	p.log.Info("Unregistered backend server",
		"name", info.Name(), "addr", info.Addr())
	return true
}

// DisconnectAll disconnects all current connected players
// in parallel and waits until all players have been disconnected.
func (p *Proxy) DisconnectAll(reason component.Component) {
	p.muP.RLock()
	players := p.playerIDs
	p.muP.RUnlock()

	var wg sync.WaitGroup
	wg.Add(len(players))
	for _, p := range players {
		go func(p *connectedPlayer) {
			defer wg.Done()
			p.Disconnect(reason)
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
	defer func() { _ = ln.Close() }()
	go func() { <-stop; _ = ln.Close() }()

	p.event.Fire(&ReadyEvent{})

	defer p.log.Info("Stopped listening for new connections")
	p.log.Info("Listening for connections", "addr", addr)

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
		go p.HandleConn(conn)
	}
}

// HandleConn handles a just-accepted client connection
// that has not had any I/O performed on it yet.
func (p *Proxy) HandleConn(raw net.Conn) {
	if p.connectionsQuota != nil && p.connectionsQuota.Blocked(netutil.Host(raw.RemoteAddr())) {
		p.log.Info("Connection exceeded rate limit, closed", "remoteAddr", raw.RemoteAddr())
		_ = raw.Close()
		return
	}

	// Create client connection
	conn := newMinecraftConn(raw, p, true)
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
	player, ok := p.playerIDs[id]
	if !ok {
		return nil // return correct nil
	}
	return player
}

// PlayerByName returns the online player by their Minecraft name (search is case-insensitive).
// Returns nil if the player was not found.
func (p *Proxy) PlayerByName(username string) Player {
	player := p.playerByName(username)
	if player == (*connectedPlayer)(nil) {
		return nil // return correct nil
	}
	return player
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
	if protocol.GreaterEqual(version.Minecraft_1_13) {
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

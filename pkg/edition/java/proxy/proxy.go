package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.minekube.com/gate/pkg/edition/java/lite"
	"go.minekube.com/gate/pkg/edition/java/proto/state"

	"github.com/go-logr/logr"
	"github.com/pires/go-proxyproto"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"golang.org/x/sync/errgroup"

	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/auth"
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/internal/addrquota"
	"go.minekube.com/gate/pkg/internal/connwrap"
	"go.minekube.com/gate/pkg/internal/reload"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.minekube.com/gate/pkg/util/validation"
)

// Proxy is Gate's Java edition Minecraft proxy.
type Proxy struct {
	log              logr.Logger
	cfg              *config.Config
	event            event.Manager
	command          command.Manager
	channelRegistrar *message.ChannelRegistrar
	authenticator    auth.Authenticator

	startTime atomic.Pointer[time.Time]

	closeMu     sync.Mutex
	startCtx    context.Context
	cancelStart context.CancelFunc
	started     bool

	muS           sync.RWMutex                 // Protects following fields
	servers       map[string]*registeredServer // registered backend servers: by lower case names
	configServers map[string]bool              // tracks which servers came from config (vs API)

	muP         sync.RWMutex                   // Protects following fields
	playerNames map[string]*connectedPlayer    // lower case usernames map
	playerIDs   map[uuid.UUID]*connectedPlayer // uuids map

	connectionsQuota *addrquota.Quota
	loginsQuota      *addrquota.Quota

	lite *lite.Lite // lite mode functionality
}

// Options are the options for a new Java edition Proxy.
type Options struct {
	// Config requires configuration
	// validated by cfg.Validate.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
	// Authenticator to authenticate users in online mode.
	// If not set, creates a default one.
	Authenticator auth.Authenticator
}

// New returns a new Proxy ready to start.
func New(options Options) (p *Proxy, err error) {
	if options.Config == nil {
		return nil, errs.ErrMissingConfig
	}
	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}
	authn := options.Authenticator
	if authn == nil {
		opts := auth.Options{
			// Default to mojang's session server
			HasJoinedURLFn: auth.CustomHasJoinedURL(options.Config.Auth.SessionServerURL.T()),
		}
		authn, err = auth.New(opts)
		if err != nil {
			return nil, fmt.Errorf("erorr creating authenticator: %w", err)
		}
	}

	p = &Proxy{
		log:              logr.Discard(), // updated by Proxy.Start
		cfg:              options.Config,
		event:            eventMgr,
		channelRegistrar: message.NewChannelRegistrar(),
		servers:          map[string]*registeredServer{},
		configServers:    map[string]bool{},
		playerNames:      map[string]*connectedPlayer{},
		playerIDs:        map[uuid.UUID]*connectedPlayer{},
		authenticator:    authn,
		lite:             lite.NewLiteWithEvent(eventMgr), // create lite mode functionality for this proxy instance
	}

	// Connection & login rate limiters
	p.initQuota(&options.Config.Quota)

	if err = p.initMeter(); err != nil {
		return nil, fmt.Errorf("error initializing meter: %w", err)
	}

	return p, nil
}

func (p *Proxy) initQuota(quota *config.Quota) {
	q := quota.Connections
	if q.Enabled {
		p.connectionsQuota = addrquota.NewQuota(q.OPS, q.Burst, q.MaxEntries)
	} else {
		p.connectionsQuota = nil
	}

	q = quota.Logins
	if q.Enabled {
		p.loginsQuota = addrquota.NewQuota(q.OPS, q.Burst, q.MaxEntries)
	} else {
		p.loginsQuota = nil
	}
}

// ErrProxyAlreadyRun is returned by Proxy.Run if the proxy instance was already run.
var ErrProxyAlreadyRun = errors.New("proxy was already run, create a new one")

// Start runs the Java edition Proxy, blocks until the proxy is
// Shutdown or an error occurred while starting.
// The Proxy is already shutdown on method return.
// Another method of stopping the Proxy is to call Shutdown.
// A Proxy can only be run once or ErrProxyAlreadyRun is returned.
func (p *Proxy) Start(ctx context.Context) error {
	p.closeMu.Lock()
	if p.started {
		p.closeMu.Unlock()
		return ErrProxyAlreadyRun
	}
	ctx, span := tracer.Start(ctx, "Proxy.Start")
	defer span.End()

	p.started = true
	now := time.Now()
	p.startTime.Store(&now)
	p.log = logr.FromContextOrDiscard(ctx)

	p.startCtx, p.cancelStart = context.WithCancel(ctx)
	ctx = p.startCtx
	defer p.cancelStart()
	p.closeMu.Unlock()

	if err := p.init(); err != nil {
		return fmt.Errorf("pre-initialization error: %w", err)
	}

	// Init "plugins" with the proxy
	if err := p.initPlugins(ctx); err != nil {
		return err
	}
	if p.cfg.Lite.Enabled {
		if err := p.initLitePlugins(ctx); err != nil {
			return err
		}
	}

	logInfo := func() {
		if p.cfg.Debug {
			p.log.Info("running in debug mode")
		}
		if p.cfg.Lite.Enabled {
			p.log.Info("running in lite mode")
		}
		if p.cfg.ProxyProtocol {
			p.log.Info("proxy protocol enabled")
		}
		if p.cfg.Auth.SessionServerURL != nil {
			p.log.Info("using custom authentication server", "url", p.cfg.Auth.SessionServerURL)
		}
	}
	logInfo()

	defer func() {
		p.Shutdown(p.config().ShutdownReason.T()) // disconnects players
	}()

	eg, ctx := errgroup.WithContext(ctx)
	listen := func(addr string) context.CancelFunc {
		lnCtx, stop := context.WithCancel(ctx)
		eg.Go(func() error {
			defer stop()
			return p.listenAndServe(lnCtx, addr)
		})
		return stop
	}

	stopLn := listen(p.cfg.Bind)

	// Listen for config reloads until we exit
	defer reload.Subscribe(p.event, func(e *javaConfigUpdateEvent) {
		*p.cfg = *e.Config
		p.initQuota(&e.Config.Quota)
		if e.PrevConfig.Bind != e.Config.Bind {
			p.closeMu.Lock()
			stopLn()
			stopLn = listen(e.Config.Bind)
			p.closeMu.Unlock()
		}
		if err := p.init(); err != nil {
			p.log.Error(err, "re-initialization error")
		}
		if e.Config.Lite.Enabled {
			// reset whole cache if routes have changed because
			// backend addrs might have moved to another route or a cacheTTL changed
			if func() bool {
				if len(e.Config.Lite.Routes) != len(e.PrevConfig.Lite.Routes) {
					return true
				}
				for i, route := range e.Config.Lite.Routes {
					if !route.Equal(&e.PrevConfig.Lite.Routes[i]) {
						return true
					}
				}
				return false
			}() {
				lite.ResetPingCache()
				p.log.Info("lite ping cache was reset")
			}
		} else {
			lite.ResetPingCache()
		}
		logInfo()
	})()

	return eg.Wait()
}

type javaConfigUpdateEvent = reload.ConfigUpdateEvent[config.Config]

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
	p.cancelStart()

	p.log.Info("shutting down the proxy...")
	shutdownTime := time.Now()
	defer func() {
		p.log.Info("finished shutdown.",
			"shutdownTime", time.Since(shutdownTime).Round(time.Microsecond).String(),
			"totalTime", time.Since(*p.startTime.Load()).Round(time.Millisecond).String())
	}()

	pre := &PreShutdownEvent{reason: reason}
	p.event.Fire(pre)
	reason = pre.Reason()

	reasonStr := new(strings.Builder)
	if reason != nil && !reflect.ValueOf(reason).IsNil() {
		err := (&legacy.Legacy{}).Marshal(reasonStr, reason)
		if err != nil {
			p.log.Error(err, "error marshal disconnect reason to legacy format")
		}
	}

	p.log.Info("disconnecting all players...", "reason", reasonStr.String())
	disconnectTime := time.Now()
	p.DisconnectAll(reason)
	p.log.Info("disconnected all players.", "time", time.Since(disconnectTime).String())

	p.log.Info("waiting for all event handlers to complete...")
	p.event.Fire(&ShutdownEvent{})
	p.event.Wait()
}

// called before starting to actually run the proxy
func (p *Proxy) init() (err error) {
	c := p.cfg

	// No need to check, nil default to mojang's session server
	p.authenticator.SetHasJoinedURLFn(auth.CustomHasJoinedURL(c.Auth.SessionServerURL.T()))

	if !c.Lite.Enabled {
		// Sync servers: register new/updated servers and unregister removed servers
		if len(c.Servers) != 0 {
			p.log.Info("syncing servers...", "count", len(c.Servers))
		}

		// Track which servers should exist after sync
		expectedServers := make(map[string]ServerInfo)

		// Process servers from config
		for name, addr := range c.Servers {
			pAddr, err := netutil.Parse(addr, "tcp")
			if err != nil {
				return fmt.Errorf("error parsing server %q address %q: %w", name, addr, err)
			}
			info := NewServerInfo(name, pAddr)
			expectedServers[strings.ToLower(name)] = info

			// Check if server is already registered
			if rs := p.Server(name); rs != nil {
				if ServerInfoEqual(rs.ServerInfo(), info) {
					// Server exists and is identical - mark as config-managed and continue
					p.muS.Lock()
					p.configServers[strings.ToLower(name)] = true
					p.muS.Unlock()
					continue
				} else {
					// Server exists but is different - unregister the old one first
					_ = p.Unregister(rs.ServerInfo())
				}
			}

			// Register the new/updated server
			_, err = p.Register(info)
			if err != nil {
				p.log.Error(err, "could not register server", "server", info)
			} else {
				// Mark as config-managed
				p.muS.Lock()
				p.configServers[strings.ToLower(name)] = true
				p.muS.Unlock()
			}
		}

		// Unregister config-managed servers that are no longer in config
		// (but preserve API-registered servers)
		currentServers := p.Servers()
		for _, rs := range currentServers {
			serverName := strings.ToLower(rs.ServerInfo().Name())
			p.muS.RLock()
			isConfigManaged := p.configServers[serverName]
			p.muS.RUnlock()
			_, shouldExist := expectedServers[serverName]

			// Only unregister if it's config-managed AND not in new config
			if isConfigManaged && !shouldExist {
				if p.Unregister(rs.ServerInfo()) {
					p.muS.Lock()
					delete(p.configServers, serverName)
					p.muS.Unlock()
					p.log.Info("unregistered server removed from config", "name", rs.ServerInfo().Name())
				}
			}
		}

		// Register builtin commands
		if c.BuiltinCommands {
			names := p.registerBuiltinCommands()
			p.log.Info("registered builtin commands", "count", len(names), "cmds", names)
		}
	}

	return nil
}

func (p *Proxy) initPlugins(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	for _, pl := range Plugins {
		start := time.Now()
		if err := pl.Init(ctx, p); err != nil {
			return fmt.Errorf("error running init hook for plugin %q: %w", pl.Name, err)
		}
		log.Info("initialized plugin", "name", pl.Name, "time", time.Since(start).Round(time.Millisecond).String())
	}
	return nil
}

func (p *Proxy) initLitePlugins(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	rt := p.lite.Runtime()
	for _, pl := range lite.Plugins {
		start := time.Now()
		if err := pl.Init(ctx, rt); err != nil {
			return fmt.Errorf("error running init hook for lite plugin %q: %w", pl.Name, err)
		}
		log.Info("initialized lite plugin", "name", pl.Name, "time", time.Since(start).Round(time.Millisecond).String())
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

// Config returns the cfg used by the Proxy.
func (p *Proxy) Config() config.Config {
	return *p.cfg
}

func (p *Proxy) config() *config.Config {
	return p.cfg
}

// Lite returns the proxy's lite mode functionality.
func (p *Proxy) Lite() *lite.Lite {
	return p.lite
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
	// Note: We don't mark API-registered servers as config-managed
	// so they won't be unregistered during config reloads

	p.log.Info("registered new server", "name", info.Name(), "addr", info.Addr())

	// Fire ServerRegisteredEvent
	p.event.Fire(&ServerRegisteredEvent{server: rs})

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
	delete(p.configServers, name) // Clean up config tracking

	p.log.Info("unregistered backend server",
		"name", info.Name(), "addr", info.Addr())

	// Fire ServerUnregisteredEvent
	p.event.Fire(&ServerUnregisteredEvent{server: info})

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
func (p *Proxy) listenAndServe(ctx context.Context, addr string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = ln.Close() }()

	p.event.Fire(&ReadyEvent{addr: addr})

	defer p.log.Info("stopped listening for new connections", "addr", addr)
	p.log.Info("listening for connections", "addr", addr)

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

		if p.cfg.ProxyProtocol {
			conn = proxyproto.NewConn(conn)
		}

		go p.HandleConn(conn)
	}
}

// HandleConn handles a just-accepted client connection
// that has not had any I/O performed on it yet.
func (p *Proxy) HandleConn(raw net.Conn) {
	if p.connectionsQuota != nil && p.connectionsQuota.Blocked(netutil.Host(raw.RemoteAddr())) {
		p.log.Info("connection exceeded rate limit, closed", "remoteAddr", raw.RemoteAddr())
		_ = raw.Close()
		return
	}

	// Create context for connection
	ctx, ok := raw.(context.Context)
	if !ok {
		ctx = context.Background()
	}
	ctx = logr.NewContext(ctx, p.log)
	ctx = trace.ContextWithSpan(ctx, trace.SpanFromContext(p.startCtx))

	// OpenTelemetry span for connection
	ctx, span := tracer.Start(ctx, "HandleConn", trace.WithAttributes(
		attribute.String("remote.host", netutil.Host(raw.RemoteAddr())),
		attribute.Stringer("local.addr", raw.LocalAddr()),
	))
	defer span.End()

	// Fire connection event
	if p.event.HasSubscriber((*ConnectionEvent)(nil)) {
		conn := &connwrap.Conn{Conn: raw}
		e := &ConnectionEvent{
			conn:     conn,
			original: conn,
		}
		p.event.Fire(e)
		if conn.Closed() || e.Connection() == nil {
			_ = conn.Close()
			p.log.V(1).Info("connection closed by ConnectionEvent subscriber", "remoteAddr", raw.RemoteAddr())
			return
		}
		raw = e.Connection()
	}

	// Create client connection
	conn, readLoop := netmc.NewMinecraftConn(
		ctx, raw, proto.ServerBound,
		time.Duration(p.cfg.ReadTimeout)*time.Millisecond,
		time.Duration(p.cfg.ConnectionTimeout)*time.Millisecond,
		p.cfg.Compression.Level,
	)
	conn.SetActiveSessionHandler(state.Handshake, newHandshakeSessionHandler(conn, &sessionHandlerDeps{
		proxy:          p,
		registrar:      p,
		configProvider: p,
		eventMgr:       p.event,
		authenticator:  p.authenticator,
		loginsQuota:    p.loginsQuota,
	}))
	readLoop()
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
	playerIDs := p.playerIDs
	p.muP.RUnlock()
	pls := make([]Player, 0, len(playerIDs))
	for _, player := range playerIDs {
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
	player, ok := p.playerNames[strings.ToLower(username)]
	if !ok {
		return nil
	}
	return player
}

func (p *Proxy) canRegisterConnection(player *connectedPlayer) bool {
	c := p.cfg
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
	c := p.cfg

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
			// we can register our connection, but this shall be uncommon anyway. :)
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
	return found
}

//
//
//
//
//
//

func (p *Proxy) ChannelRegistrar() *message.ChannelRegistrar {
	return p.channelRegistrar
}

//
//
//
//
//

// MessageSink is a message sink.
type MessageSink interface {
	// SendMessage sends a message component to the entity.
	SendMessage(msg component.Component, opts ...command.MessageOption) error
}

// BroadcastMessage broadcasts a message to all given sinks (e.g. Player).
func BroadcastMessage(sinks []MessageSink, msg component.Component) {
	for _, sink := range sinks {
		go func(s MessageSink) { _ = s.SendMessage(msg) }(sink)
	}
}

//
//
//

func withConnectionTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(cfg.ConnectionTimeout)*time.Millisecond)
}

type (
	configProvider interface {
		config() *config.Config
	}
)

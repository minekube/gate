package geyser

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pires/go-proxyproto"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/managed"
	"go.minekube.com/gate/pkg/edition/java/lite"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/netutil"
	"go.minekube.com/gate/pkg/util/uuid"
)

type managedRunner interface {
	EnsureKey(context.Context) error
	Start(context.Context) error
	Stop()
	Done() <-chan struct{}
	Err() error
}

type javaManagedRunner struct {
	runner *managed.Runner
}

func newJavaManagedRunner(cfg *config.Config) *javaManagedRunner {
	return &javaManagedRunner{runner: managed.New(cfg)}
}

func (r *javaManagedRunner) EnsureKey(ctx context.Context) error {
	return r.runner.EnsureKey(ctx)
}

func (r *javaManagedRunner) Start(ctx context.Context) error {
	jar, err := r.runner.Ensure(ctx)
	if err != nil {
		return fmt.Errorf("managed java geyser ensure failed: %w", err)
	}
	if err := r.runner.Start(ctx, jar); err != nil {
		return fmt.Errorf("managed java geyser start failed: %w", err)
	}
	return nil
}

func (r *javaManagedRunner) Stop() {
	r.runner.Stop()
}

func (r *javaManagedRunner) Done() <-chan struct{} { return r.runner.Done() }

func (r *javaManagedRunner) Err() error { return r.runner.Err() }

func newManagedRunner(cfg *config.Config) (managedRunner, error) {
	managedConfig := cfg.GetManaged()
	switch managedConfig.Engine {
	case "", config.ManagedEngineGeyserlite:
		return newLiteManagedRunner(cfg), nil
	case config.ManagedEngineJava:
		return newJavaManagedRunner(cfg), nil
	default:
		return nil, fmt.Errorf("unknown managed geyser engine %q (want %q or %q)",
			managedConfig.Engine, config.ManagedEngineGeyserlite, config.ManagedEngineJava)
	}
}

// Integration provides Geyser integration for Gate.
type Integration struct {
	ctx            context.Context
	cancel         context.CancelFunc
	log            logr.Logger
	proxy          *proxy.Proxy
	config         *config.Config
	floodgate      *floodgate.Floodgate
	profileManager *ProfileManager
	connections    map[net.Addr]*GeyserConnection
	mu             sync.RWMutex
	unsubs         []func()
	unregisterHook func()
	manager        managedRunner
	runtimeErr     chan error
	runtimeErrOnce sync.Once
}

// GeyserConnection represents a connection from Geyser.
type GeyserConnection struct {
	context.Context
	net.Conn
	*floodgate.BedrockData
	OriginalHost string
	closeCb      func()
}

func (c *GeyserConnection) Close() error {
	c.closeCb()
	return c.Conn.Close()
}

// NewIntegration creates a new Geyser integration.
func NewIntegration(ctx context.Context, p *proxy.Proxy, cfg *config.Config) (*Integration, error) {
	if cfg.FloodgateKeyPath == "" {
		return nil, fmt.Errorf("floodgate key path is required for Bedrock support")
	}

	logr.FromContextOrDiscard(ctx).Info("bedrock config loaded",
		"floodgateKeyPath", cfg.FloodgateKeyPath,
		"geyserListenAddr", cfg.GeyserListenAddr,
		"usernameFormat", cfg.UsernameFormat)

	ctx2, cancel := context.WithCancel(ctx)
	integration := &Integration{
		ctx:            ctx2,
		cancel:         cancel,
		log:            logr.FromContextOrDiscard(ctx).WithName("geyser"),
		proxy:          p,
		config:         cfg,
		profileManager: NewProfileManager(),
		connections:    make(map[net.Addr]*GeyserConnection),
		runtimeErr:     make(chan error, 1),
	}

	managedConfig := cfg.GetManaged()
	if managedConfig.Enabled {
		configCopy := *cfg
		configCopy.Managed = &managedConfig
		manager, err := newManagedRunner(&configCopy)
		if err != nil {
			cancel()
			return nil, err
		}
		integration.manager = manager

		// In managed mode, ensure key exists before reading it
		if err := integration.manager.EnsureKey(ctx); err != nil {
			return nil, fmt.Errorf("failed to ensure floodgate key: %w", err)
		}
	}

	// Read floodgate key (now guaranteed to exist if in managed mode)
	keyBytes, err := os.ReadFile(cfg.FloodgateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read floodgate key: %w", err)
	}

	fg, err := floodgate.NewFloodgate(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize floodgate: %w", err)
	}
	integration.floodgate = fg
	if cfg.BackendFloodgate.Enabled {
		integration.unregisterHook = p.SetBackendHandshakeAddresser(integration)
	}

	return integration, nil
}

// Start starts the Geyser integration listener.
func (i *Integration) Start() error {
	eventMgr := i.proxy.Event()

	// Subscribe to proxy events
	// High priority to ensure that we handle Bedrock players before other handlers.
	const priority = math.MaxInt - 100
	unsubPre := event.Subscribe(eventMgr, priority, i.onPreLogin)
	unsubProf := event.Subscribe(eventMgr, priority, i.onGameProfile)
	i.unsubs = append(i.unsubs, unsubPre, unsubProf)

	ln, err := i.listen()
	if err != nil {
		return err
	}
	go func() {
		if err := i.serve(ln); err != nil {
			i.reportRuntimeError(fmt.Errorf("geyser listener failed: %w", err))
		}
	}()

	// If managed mode enabled, ensure and start Geyser Standalone
	if i.manager != nil {
		if err := i.manager.Start(i.ctx); err != nil {
			_ = ln.Close()
			return fmt.Errorf("managed geyser start failed: %w", err)
		}
		i.watchManagedRuntime()
	}

	i.log.Info("geyser integration started", "addr", i.config.GeyserListenAddr)
	return nil
}

// RuntimeErrors reports an unexpected failure of the managed Bedrock runtime
// or the local Geyser listener. Consumers must stop the enclosing proxy when
// they receive an error: a healthy Java/TCP listener alone does not prove that
// the public Bedrock UDP listener is still accepting players.
func (i *Integration) RuntimeErrors() <-chan error { return i.runtimeErr }

// Done closes when the integration is intentionally stopped. Runtime-error
// observers use it to leave cleanly during a reload instead of waiting for an
// error that is never meant to arrive.
func (i *Integration) Done() <-chan struct{} { return i.ctx.Done() }

func (i *Integration) watchManagedRuntime() {
	done := i.manager.Done()
	if done == nil {
		return
	}
	go func() {
		select {
		case <-i.ctx.Done():
			return
		case <-done:
			if i.ctx.Err() != nil {
				return
			}
			err := i.manager.Err()
			if err == nil {
				err = fmt.Errorf("managed geyserlite exited unexpectedly")
			}
			i.reportRuntimeError(fmt.Errorf("managed geyser runtime failed: %w", err))
		}
	}()
}

func (i *Integration) reportRuntimeError(err error) {
	if err == nil || i.ctx.Err() != nil {
		return
	}
	i.runtimeErrOnce.Do(func() {
		i.log.Error(err, "bedrock runtime failed")
		i.runtimeErr <- err
	})
}

// Stop stops the Geyser integration listener and unsubscribes events.
func (i *Integration) Stop() {
	// Cancel listener context
	if i.cancel != nil {
		i.cancel()
	}
	// Unsubscribe events
	for _, u := range i.unsubs {
		if u != nil {
			u()
		}
	}
	i.unsubs = nil
	// Close any tracked connections
	i.mu.Lock()
	for addr, c := range i.connections {
		_ = c.Close()
		delete(i.connections, addr)
	}
	i.mu.Unlock()
	// Stop managed process if running
	if i.manager != nil {
		i.manager.Stop()
	}
	if i.unregisterHook != nil {
		i.unregisterHook()
		i.unregisterHook = nil
	}
}

func (i *Integration) listen() (net.Listener, error) {
	if i.ctx.Err() != nil {
		return nil, i.ctx.Err()
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(i.ctx, "tcp", i.config.GeyserListenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", i.config.GeyserListenAddr, err)
	}
	return ln, nil
}

func (i *Integration) serve(ln net.Listener) error {
	defer func() { _ = ln.Close() }()

	ctx, cancel := context.WithCancel(i.ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = ln.Close() }()

	defer i.log.Info("stopped listening for geyser connections", "addr", i.config.GeyserListenAddr)
	i.log.Info("listening for geyser connections", "addr", i.config.GeyserListenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errs.IsConnClosedErr(err) {
				return nil
			}
			return fmt.Errorf("error accepting connection: %w", err)
		}

		go i.handleConnection(conn)
	}
}

type bedrockContext struct{}

var bedrockContextKey = bedrockContext{}

func withBedrockContext(ctx context.Context, geyserConn *GeyserConnection) context.Context {
	return context.WithValue(ctx, bedrockContextKey, geyserConn)
}

// FromContext safely retrieves a Geyser connection associated with a player.Context().
func FromContext(ctx context.Context) (*GeyserConnection, bool) {
	v, ok := ctx.Value(bedrockContextKey).(*GeyserConnection)
	if !ok {
		return nil, false
	}
	return v, true
}

func (i *Integration) handleConnection(conn net.Conn) {
	// Wrap connection with proxy protocol support
	geyserConn := &GeyserConnection{
		Conn: proxyproto.NewConn(conn),
		closeCb: func() {
			_ = conn.Close()
		},
	}
	geyserConn.Context = withBedrockContext(i.ctx, geyserConn)

	i.mu.Lock()
	i.connections[geyserConn.RemoteAddr()] = geyserConn
	i.mu.Unlock()

	// Handle the connection through Gate's Java proxy
	i.proxy.HandleLoopbackConn(geyserConn.Context, geyserConn)
}

func (i *Integration) onPreLogin(e *proxy.PreLoginEvent) {
	// Check if this is a Bedrock player connection
	geyserConn, isGeyser := FromContext(e.Conn().Context())
	if !isGeyser {
		return // Not a Geyser connection
	}

	// Extract Bedrock data from hostname
	if hostname := e.Conn().VirtualHost(); hostname != nil {
		originalHost, bedrockData, err := i.floodgate.ReadHostname(hostname.String())
		if err != nil || originalHost == "" || bedrockData == nil {
			// The raw hostname may embed Floodgate identity data and is never logged.
			i.log.Info("disconnecting bedrock player: failed to read hostname", "error", err)
			e.Deny(&component.Text{Content: "Failed to read bedrock hostname"})
			return
		}

		geyserConn.BedrockData = bedrockData
		geyserConn.OriginalHost = originalHost
		e.SetVirtualHost(cleanedVirtualHost(hostname, originalHost))

		// Force offline mode for Bedrock players (Floodgate handles auth)
		e.ForceOfflineMode()

		// No raw identity (username, XUID, linked Java identity) in logs.
		i.log.Info("bedrock player connecting",
			"device_os", bedrockData.DeviceOS,
			"language", bedrockData.Language,
			"original_host", originalHost)
	}
}

func (i *Integration) onGameProfile(e *proxy.GameProfileRequestEvent) {
	// Check if this is a Bedrock player
	geyserConn, isGeyser := FromContext(e.Conn().Context())
	if !isGeyser || geyserConn.BedrockData == nil {
		return
	}

	bedrockData := geyserConn.BedrockData

	// Generate UUID from XUID
	uid, err := bedrockData.JavaUuid()
	if err != nil || uid == uuid.Nil {
		i.log.Info("disconnecting bedrock player: failed to get UUID from XUID", "error", err)
		geyserConn.Close()
		return
	}

	// Format username to avoid conflicts with Java players
	formattedName := bedrockData.Username
	if i.config.UsernameFormat != "" {
		formattedName = fmt.Sprintf(i.config.UsernameFormat, bedrockData.Username)
	}
	formattedName = javaCompatibleUsername(formattedName)

	// Opt-in linked Java identity, gated on the backendFloodgate trust switch
	// (default off). Two sources, in priority order:
	//
	//  1. AES-authenticated Floodgate handshake triplet. Only parties holding
	//     the shared Floodgate key can produce it; it needs no network and is
	//     the authoritative data for this connection. It is cross-checked so a
	//     link can only ever be applied to the Bedrock connection it was
	//     issued for: the triplet's Bedrock UUID must equal this connection's
	//     own Floodgate bedrock UUID (new UUID(0, xuid)).
	//  2. GeyserMC global link API fallback (used when the handshake carries
	//     no triplet, e.g. standalone Geyser). This is the official Floodgate
	//     linking service (GlobalPlayerLinking, enable-global-linking default
	//     true) that backend Floodgate plugins use in production; HTTPS,
	//     operated by GeyserMC, same trust basis as the skin API below.
	//     Fail-closed: an API error leaves the XUID-derived identity.
	//
	// The unauthenticated GeyserMC hint is never consulted outside this opt-in
	// boundary, and linked identity from signed authoritative provenance (the
	// verified Bedrock principal on the Connect proposal path) is applied
	// separately and is untouched.
	if i.config.BackendFloodgate.Enabled {
		if link := floodgate.ParseLinkedPlayer(bedrockData.LinkedPlayer); link != nil &&
			link.BedrockUUID == bedrockData.FloodgateJavaUuid() {
			uid = link.JavaUUID
			formattedName = javaCompatibleUsername(link.JavaUsername)
		} else if linked, err := i.profileManager.GetLinkedAccount(bedrockData.Xuid); err == nil &&
			linked != nil && linked.JavaID != uuid.Nil {
			uid = linked.JavaID
			formattedName = javaCompatibleUsername(linked.JavaName)
		}
	}

	// Create base game profile
	gameProfile := profile.GameProfile{
		Name: formattedName,
		ID:   uid,
	}

	// Try to get skin from GeyserMC API
	if skin, err := i.profileManager.GetSkin(bedrockData.Xuid); err == nil && skin != nil {
		gameProfile.Properties = append(gameProfile.Properties, profile.Property{
			Name:      "textures",
			Value:     skin.Value,
			Signature: skin.Signature,
		})
		i.log.V(1).Info("applied bedrock skin", "texture_id", skin.TextureID)
	}

	e.SetGameProfile(gameProfile)
}

// javaCompatibleUsername makes a Bedrock gamertag safe for the Java profile
// boundary. Bedrock names may contain spaces and other characters that modern
// Java servers reject, while Java profile names are limited to 16 ASCII
// letters, digits, and underscores.
func javaCompatibleUsername(name string) string {
	const maxJavaUsernameLen = 16

	var normalized strings.Builder
	normalized.Grow(min(len(name), maxJavaUsernameLen))
	for _, r := range name {
		if normalized.Len() == maxJavaUsernameLen {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			normalized.WriteByte(byte(r))
		default:
			normalized.WriteByte('_')
		}
	}

	if normalized.Len() == 0 {
		return "_"
	}
	return normalized.String()
}

func cleanedVirtualHost(current net.Addr, originalHost string) net.Addr {
	network := "tcp"
	currentPort := uint16(0)
	if current != nil {
		network = current.Network()
		currentPort = virtualHostPort(current)
	}
	host, port := splitOriginalHostPort(originalHost)
	if port == 0 {
		port = currentPort
	}
	host = lite.ClearVirtualHost(host)
	if port == 0 {
		return netutil.NewAddr(host, network)
	}
	return netutil.NewAddr(net.JoinHostPort(host, strconv.Itoa(int(port))), network)
}

func virtualHostPort(addr net.Addr) uint16 {
	_, port := netutil.HostPort(addr)
	if port != 0 {
		return port
	}
	host := addr.String()
	if !strings.Contains(host, "\x00") {
		return 0
	}
	idx := strings.LastIndex(host, ":")
	if idx == -1 || idx == len(host)-1 {
		return 0
	}
	portInt, err := strconv.Atoi(host[idx+1:])
	if err != nil || portInt <= 0 || portInt > 65535 {
		return 0
	}
	return uint16(portInt)
}

func splitOriginalHostPort(originalHost string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(originalHost)
	if err == nil {
		port, err := strconv.Atoi(portStr)
		if err == nil && port > 0 && port <= 65535 {
			return host, uint16(port)
		}
		return host, 0
	}
	if strings.HasPrefix(originalHost, "[") && strings.HasSuffix(originalHost, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(originalHost, "["), "]"), 0
	}
	return originalHost, 0
}

// BackendHandshakeAddr re-attaches verified Floodgate hostname data for
// allowlisted backend Floodgate plugins.
func (i *Integration) BackendHandshakeAddr(defaultServerAddress string, player proxy.Player, target proxy.RegisteredServer) (string, error) {
	if i == nil || i.config == nil || !i.config.BackendFloodgate.Enabled {
		return defaultServerAddress, nil
	}
	if strings.ContainsRune(defaultServerAddress, '\x00') {
		return "", fmt.Errorf("refusing backend Floodgate hostname prefix containing NUL")
	}
	if !i.backendFloodgateAllowed(target) {
		return defaultServerAddress, nil
	}

	geyserConn, ok := FromContext(player.Context())
	if !ok || geyserConn.BedrockData == nil {
		return defaultServerAddress, nil
	}
	if i.floodgate == nil {
		return "", fmt.Errorf("backend Floodgate is enabled but Floodgate is not initialized")
	}

	encoded, err := i.floodgate.WriteHostname(defaultServerAddress, geyserConn.BedrockData)
	if err != nil {
		return "", fmt.Errorf("failed to encode backend Floodgate hostname: %w", err)
	}
	return encoded, nil
}

func (i *Integration) backendFloodgateAllowed(target proxy.RegisteredServer) bool {
	if target == nil || target.ServerInfo() == nil {
		return false
	}
	targetName := strings.ToLower(target.ServerInfo().Name())
	for _, name := range i.config.BackendFloodgate.AllowedServers {
		if strings.ToLower(name) == targetName {
			return true
		}
	}
	return false
}

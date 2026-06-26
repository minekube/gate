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
	manager        managedRunner
}

// GeyserConnection represents a connection from Geyser.
type GeyserConnection struct {
	context.Context
	net.Conn
	*floodgate.BedrockData
	OriginalHost  string
	LinkedAccount *LinkedAccountResult
	closeCb       func()
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

	return integration, nil
}

// Start starts the Geyser integration listener.
func (i *Integration) Start() error {
	eventMgr := i.proxy.Event()

	// Set ourselves as the global handshake addresser
	i.proxy.SetGlobalHandshakeAddresser(i)

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
			i.log.Error(err, "geyser listener failed")
		}
	}()

	// If managed mode enabled, ensure and start Geyser Standalone
	if i.manager != nil {
		if err := i.manager.Start(i.ctx); err != nil {
			_ = ln.Close()
			return fmt.Errorf("managed geyser start failed: %w", err)
		}
	}

	i.log.Info("geyser integration started", "addr", i.config.GeyserListenAddr)
	return nil
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
	i.proxy.HandleConn(geyserConn)
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
			i.log.Info("disconnecting bedrock player: failed to read hostname",
				"error", err, "hostname", hostname.String())
			e.Deny(&component.Text{Content: "Failed to read bedrock hostname"})
			return
		}

		geyserConn.BedrockData = bedrockData
		geyserConn.OriginalHost = originalHost
		e.SetVirtualHost(cleanedVirtualHost(hostname, originalHost))

		// Force offline mode for Bedrock players (Floodgate handles auth)
		e.ForceOfflineMode()

		i.log.Info("bedrock player connecting",
			"username", bedrockData.Username,
			"xuid", bedrockData.Xuid,
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
		i.log.Info("disconnecting bedrock player: failed to get UUID from XUID",
			"error", err, "xuid", bedrockData.Xuid)
		geyserConn.Close()
		return
	}

	// Format username to avoid conflicts with Java players
	formattedName := bedrockData.Username
	if i.config.UsernameFormat != "" {
		formattedName = fmt.Sprintf(i.config.UsernameFormat, bedrockData.Username)
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
		i.log.V(1).Info("applied bedrock skin", "username", formattedName, "texture_id", skin.TextureID)
	}

	// Check for linked Java account
	if linkedAccount, err := i.profileManager.GetLinkedAccount(bedrockData.Xuid); err == nil && linkedAccount != nil && linkedAccount.JavaID != uuid.Nil {
		geyserConn.LinkedAccount = linkedAccount
		bedrockData.LinkedPlayer = linkedAccount.JavaName

		// Use linked Java account details
		i.log.Info("bedrock player using linked java account",
			"bedrock_name", bedrockData.Username,
			"java_name", linkedAccount.JavaName,
			"java_uuid", linkedAccount.JavaID)

		gameProfile.ID = linkedAccount.JavaID
		gameProfile.Name = linkedAccount.JavaName
		// TODO: Get skin for linked Java account if needed
	}

	e.SetGameProfile(gameProfile)
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

// HandshakeAddr implements the proxy.HandshakeAddresser interface.
// It adds the Floodgate data back to the virtual host when forwarding to backend servers.
func (i *Integration) HandshakeAddr(vHost string, player proxy.Player) string {
	geyserConn, ok := FromContext(player.Context())
	if !ok || geyserConn.BedrockData == nil {
		// Not a Geyser connection, return original
		return vHost
	}

	// Encode the original host and bedrock data back into the virtual host
	encoded, err := i.floodgate.WriteHostname(geyserConn.OriginalHost, geyserConn.BedrockData)
	if err != nil {
		i.log.Error(err, "failed to encode floodgate hostname for backend connection")
		return vHost
	}

	return encoded
}

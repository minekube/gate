package geyser

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pires/go-proxyproto"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/bedrock/config"
	"go.minekube.com/gate/pkg/edition/bedrock/geyser/floodgate"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Integration provides Geyser integration for Gate.
type Integration struct {
	ctx            context.Context
	log            logr.Logger
	proxy          *proxy.Proxy
	config         *config.Config
	floodgate      *floodgate.Floodgate
	profileManager *ProfileManager
	connections    map[net.Addr]*GeyserConnection
	mu             sync.RWMutex
}

// GeyserConnection represents a connection from Geyser.
type GeyserConnection struct {
	net.Conn
	*floodgate.BedrockData
	closeCb func()
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
	// Read floodgate key
	keyBytes, err := os.ReadFile(cfg.FloodgateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read floodgate key: %w", err)
	}

	fg, err := floodgate.NewFloodgate(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize floodgate: %w", err)
	}

	integration := &Integration{
		ctx:            ctx,
		log:            logr.FromContextOrDiscard(ctx).WithName("geyser"),
		proxy:          p,
		config:         cfg,
		floodgate:      fg,
		profileManager: NewProfileManager(),
		connections:    make(map[net.Addr]*GeyserConnection),
	}

	return integration, nil
}

// Start starts the Geyser integration listener.
func (i *Integration) Start() error {
	eventMgr := i.proxy.Event()

	// Subscribe to proxy events
	event.Subscribe(eventMgr, 0, i.onPreLogin)
	event.Subscribe(eventMgr, 0, i.onGameProfile)

	// Start listening for Geyser connections
	go func() {
		if err := i.listenAndServe(); err != nil {
			i.log.Error(err, "geyser listener failed")
		}
	}()

	i.log.Info("geyser integration started", "addr", i.config.GeyserListenAddr)
	return nil
}

func (i *Integration) listenAndServe() error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(i.ctx, "tcp", i.config.GeyserListenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", i.config.GeyserListenAddr, err)
	}
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

func (i *Integration) handleConnection(conn net.Conn) {
	// Wrap connection with proxy protocol support
	geyserConn := &GeyserConnection{
		Conn: proxyproto.NewConn(conn),
		closeCb: func() {
			_ = conn.Close()
		},
	}

	i.mu.Lock()
	i.connections[geyserConn.RemoteAddr()] = geyserConn
	i.mu.Unlock()

	// Handle the connection through Gate's Java proxy
	i.proxy.HandleConn(geyserConn)
}

func (i *Integration) onPreLogin(e *proxy.PreLoginEvent) {
	// Check if this is a Bedrock player connection
	geyserConn, isGeyser := i.getGeyserConnection(e.Conn().RemoteAddr())
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
	geyserConn, isGeyser := i.getGeyserConnection(e.Conn().RemoteAddr())
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

// getGeyserConnection safely retrieves a Geyser connection.
func (i *Integration) getGeyserConnection(addr net.Addr) (*GeyserConnection, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, conn := range i.connections {
		if conn.RemoteAddr().String() == addr.String() {
			return conn, true
		}
	}
	return nil, false
}

// isGeyserConnection checks if an address belongs to a Geyser connection.
func (i *Integration) isGeyserConnection(addr net.Addr) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	for _, conn := range i.connections {
		if conn.RemoteAddr().String() == addr.String() {
			return true
		}
	}
	return false
}

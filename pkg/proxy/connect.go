package proxy

import (
	"errors"
	"fmt"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/internal/util/quotautil"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/util/errs"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/zap"
	"net"
	"strings"
	"sync"
)

// connect is the connections manager for the Proxy.
type connect struct {
	proxy            *Proxy
	connectionsQuota *quotautil.Quota
	loginsQuota      *quotautil.Quota

	mu    sync.RWMutex                   // Protects following fields
	names map[string]*connectedPlayer    // lower case usernames map
	ids   map[uuid.UUID]*connectedPlayer // uuids map
}

func newConnect(proxy *Proxy) *connect {
	c := &connect{
		proxy: proxy,
		names: map[string]*connectedPlayer{},
		ids:   map[uuid.UUID]*connectedPlayer{},
	}
	quota := proxy.config.Quota.Connections
	if quota.Enabled {
		c.connectionsQuota = quotautil.NewQuota(quota.OPS, quota.Burst, quota.MaxEntries)
	}
	quota = proxy.config.Quota.Logins
	if quota.Enabled {
		c.loginsQuota = quotautil.NewQuota(quota.OPS, quota.Burst, quota.MaxEntries)
	}
	return c
}

func (c *connect) DisconnectAll(reason component.Component) {
	c.mu.RLock()
	players := c.ids
	c.mu.RUnlock()
	for _, p := range players {
		p.Disconnect(reason)
	}
}

// listenAndServe starts listening for connections on addr until closed channel receives.
func (c *connect) listenAndServe(addr string, stop <-chan struct{}) error {
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

	c.proxy.event.Fire(&ReadyEvent{})

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
		go c.handleRawConn(conn)
	}
}

// handleRawConn handles a just-accepted connection that
// has not had any I/O performed on it yet.
func (c *connect) handleRawConn(raw net.Conn) {
	if c.connectionsQuota != nil && c.connectionsQuota.Blocked(raw.RemoteAddr()) {
		_ = raw.Close()
		zap.L().Info("A connection was exceeded the rate limit", zap.Stringer("remoteAddr", raw.RemoteAddr()))
		return
	}

	// Create client connection
	conn := newMinecraftConn(raw, c.proxy, true, func() []zap.Field {
		return []zap.Field{zap.Bool("player", true)}
	})
	conn.setSessionHandler0(newHandshakeSessionHandler(conn))
	// Read packets in loop
	conn.readLoop()
}

// PlayerCount returns the number of players on the proxy.
func (c *connect) PlayerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.ids)
}

// Players returns all players on the proxy.
func (c *connect) Players() []Player {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pls := make([]Player, 0, len(c.ids))
	for _, player := range c.ids {
		pls = append(pls, player)
	}
	return pls
}

// Player returns the online player by their Minecraft id.
// Returns nil if the player was not found.
func (c *connect) Player(id uuid.UUID) Player {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ids[id]
}

// Player returns the online player by their Minecraft name (search is case-insensitive).
// Returns nil if the player was not found.
func (c *connect) PlayerByName(username string) Player {
	return c.playerByName(username)
}
func (c *connect) playerByName(username string) *connectedPlayer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.names[strings.ToLower(username)]
}

func (c *connect) canRegisterConnection(player *connectedPlayer) bool {
	cfg := c.config()
	if cfg.OnlineMode && cfg.OnlineModeKickExistingPlayers {
		return true
	}
	lowerName := strings.ToLower(player.Username())
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.names[lowerName] == nil && c.ids[player.Id()] == nil
}

// Attempts to register the connection with the proxy.
func (c *connect) registerConnection(player *connectedPlayer) bool {
	lowerName := strings.ToLower(player.Username())
	cfg := c.config()

retry:
	c.mu.Lock()
	if cfg.OnlineModeKickExistingPlayers {
		existing, ok := c.ids[player.Id()]
		if ok {
			// Make sure we disconnect existing duplicate
			// player connection before we register the new one.
			//
			// Disconnecting the existing connection will call c.unregisterConnection in the
			// teardown needing the c.mu.Lock() so we unlock.
			c.mu.Unlock()
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
		_, exists := c.names[lowerName]
		if exists {
			return false
		}
		_, exists = c.ids[player.Id()]
		if exists {
			return false
		}
	}

	c.ids[player.Id()] = player
	c.names[lowerName] = player
	c.mu.Unlock()
	return true
}

// unregisters a connected player
func (c *connect) unregisterConnection(player *connectedPlayer) (found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, found = c.ids[player.Id()]
	delete(c.names, strings.ToLower(player.Username()))
	delete(c.ids, player.Id())
	// TODO c.s.bossBarManager.onDisconnect(player)?
	return found
}

func (c *connect) config() *config.Config {
	return c.proxy.config
}

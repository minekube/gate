package proxy

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Built-in Geyser/Floodgate plugin that mirrors gate-geyser behavior while being compiled in.
// Enabled when cfg.Floodgate.Enabled is true and cfg.Floodgate.KeyFile is set.

type fgPlugin struct {
	ctx      context.Context
	proxy    *Proxy
	keyFile  string
	namePref string

	mu    sync.RWMutex
	conns map[net.Addr]*fgConn
}

type fgConn struct {
	net.Conn
	bedrock bedrockData
	closeCb func()
}

func (c *fgConn) Close() error { c.closeCb(); return c.Conn.Close() }

func init() {
	// Register built-in plugin hook but activate only if config enabled
	Plugins = append(Plugins, Plugin{
		Name: "BuiltinFloodgate",
		Init: func(ctx context.Context, p *Proxy) error {
			cfg := p.config()
			if !cfg.Floodgate.Enabled || cfg.Floodgate.KeyFile == "" {
				return nil
			}

			// Ensure key file is readable; decryption is handled earlier in handshake path
			if _, err := os.ReadFile(cfg.Floodgate.KeyFile); err != nil {
				return err
			}

			pl := &fgPlugin{
				ctx:      ctx,
				proxy:    p,
				keyFile:  cfg.Floodgate.KeyFile,
				namePref: cfg.Floodgate.UsernamePrefix,
				conns:    make(map[net.Addr]*fgConn),
			}
			event.Subscribe(p.Event(), 0, pl.onGameProfile)
			return nil
		},
	})
}

func (p *fgPlugin) onGameProfile(e *GameProfileRequestEvent) {
	// Use precomputed JavaName from Floodgate hostname if present; fallback to current name
	host := e.Conn().VirtualHost().String()
	res, err := detectFloodgate(host, p.proxy.cfg)
	if err != nil || !res.Verified {
		return
	}
	uid := uuid.OfflinePlayerUUID(res.JavaName)
	gameProfile := profile.GameProfile{
		Name: res.JavaName,
		ID:   uid,
	}
	e.SetGameProfile(gameProfile)
}


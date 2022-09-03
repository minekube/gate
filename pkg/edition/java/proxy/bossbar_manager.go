package proxy

import (
	"sync"

	"go.minekube.com/gate/pkg/edition/java/bossbar"
	"go.minekube.com/gate/pkg/util/uuid"
)

type (
	// BossBarManager manages boss bars.
	// Viewers are automatically removed from boss bars when they are not online anymore.
	// It is safe for concurrent use.
	BossBarManager interface {
		// Register registers the given boss bar.
		Register(bossbar.BossBar)
		// Unregister removes a boss bar from the manager.
		// It does not remove the viewers from the boss bar.
		Unregister(bossbar.BossBar)
	}
	bossBarManager struct {
		playerProvider
		sync.RWMutex
		bars map[uuid.UUID]bossbar.BossBar
	}
)

func newBossBarManager(p playerProvider) *bossBarManager {
	return &bossBarManager{
		playerProvider: p,
		bars:           make(map[uuid.UUID]bossbar.BossBar),
	}
}

// BossBarManager returns the boss bar manager.
func (p *Proxy) BossBarManager() BossBarManager {
	return p.bossBarManager
}

func (b *bossBarManager) Register(bar bossbar.BossBar) {
	b.Lock()
	defer b.Unlock()
	_, ok := b.bars[bar.ID()]
	if !ok {
		b.bars[bar.ID()] = bar
	}
	b.Unlock()

	b.sync(bar)
}

// sync removes viewers from the boss bar that are not online anymore.
func (b *bossBarManager) sync(bar bossbar.BossBar) {
	for _, viewer := range bar.Viewers() {
		if b.playerProvider.Player(viewer.ID()) != nil {
			_ = bar.RemoveViewer(viewer)
		}
	}
}

func (b *bossBarManager) Unregister(bar bossbar.BossBar) {
	b.Lock()
	defer b.Unlock()
	delete(b.bars, bar.ID())
}

func (b *bossBarManager) removeViewer(viewer bossbar.Viewer) {
	b.Lock()
	bars := b.bars
	b.Unlock()
	for _, bar := range bars {
		_ = bar.RemoveViewer(viewer)
	}
}

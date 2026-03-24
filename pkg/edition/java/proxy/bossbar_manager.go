package proxy

import (
	"sync"

	"go.minekube.com/gate/pkg/edition/java/bossbar"
	bossbarpacket "go.minekube.com/gate/pkg/edition/java/proto/packet/bossbar"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/uuid"
)

// bossBarManager handles dropping and resending boss bar packets on versions 1.20.2 and newer
// because the client deletes all boss bars during the config phase, and sending update packets
// would cause the client to be disconnected.
type bossBarManager struct {
	player *connectedPlayer

	mu          sync.Mutex
	bossBars    map[uuid.UUID]bossbar.BossBar // keyed by boss bar ID
	dropPackets bool
}

func newBossBarManager(player *connectedPlayer) *bossBarManager {
	return &bossBarManager{
		player:   player,
		bossBars: make(map[uuid.UUID]bossbar.BossBar),
	}
}

// shouldManage returns true if this player's protocol version requires boss bar management.
func (m *bossBarManager) shouldManage() bool {
	return m.player.Protocol().GreaterEqual(version.Minecraft_1_20_2)
}

// RegisterBossBar registers a proxy-level boss bar with this player.
// This should be called when a boss bar is shown to the player.
func (m *bossBarManager) RegisterBossBar(bar bossbar.BossBar) {
	if !m.shouldManage() {
		return
	}
	m.mu.Lock()
	m.bossBars[bar.ID()] = bar
	m.mu.Unlock()
}

// UnregisterBossBar unregisters a proxy-level boss bar from this player.
// This should be called when a boss bar is hidden from the player.
func (m *bossBarManager) UnregisterBossBar(bar bossbar.BossBar) {
	if !m.shouldManage() {
		return
	}
	m.mu.Lock()
	delete(m.bossBars, bar.ID())
	m.mu.Unlock()
}

// WritePacket writes a boss bar packet to the player, respecting the drop state.
// Returns true if the packet was written (or dropped intentionally), false on error.
func (m *bossBarManager) WritePacket(p *bossbarpacket.BossBar) bool {
	m.mu.Lock()
	drop := m.dropPackets
	m.mu.Unlock()

	if drop {
		// Intentionally drop the packet during server transition
		return true
	}

	return m.player.WritePacket(p) == nil
}

// StartDropping prevents boss bar update packets from being sent to the player.
// This should be called when the player enters config state for server switching.
func (m *bossBarManager) StartDropping() {
	if !m.shouldManage() {
		return
	}
	m.mu.Lock()
	m.dropPackets = true
	m.mu.Unlock()
}

// SendBossBars re-creates all proxy-level boss bars for the player and stops dropping packets.
// This should be called after the player has joined a new server.
func (m *bossBarManager) SendBossBars() {
	if !m.shouldManage() {
		return
	}

	m.mu.Lock()
	bars := make([]bossbar.BossBar, 0, len(m.bossBars))
	for _, bar := range m.bossBars {
		bars = append(bars, bar)
	}
	m.dropPackets = false
	m.mu.Unlock()

	// Re-add the player as a viewer to each boss bar, which will send the ADD packet
	for _, bar := range bars {
		// The boss bar package handles creating the add packet
		_ = bar.AddViewer(m.player)
	}
}

// IsDropping returns true if boss bar packets are currently being dropped.
func (m *bossBarManager) IsDropping() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropPackets
}

// Implement bossbar.ManagedViewer interface on connectedPlayer

// RegisterBossBar implements bossbar.ManagedViewer.
func (p *connectedPlayer) RegisterBossBar(bar bossbar.BossBar) {
	p.bossBarManager.RegisterBossBar(bar)
}

// UnregisterBossBar implements bossbar.ManagedViewer.
func (p *connectedPlayer) UnregisterBossBar(bar bossbar.BossBar) {
	p.bossBarManager.UnregisterBossBar(bar)
}

// WriteBossBarPacket implements bossbar.ManagedViewer.
func (p *connectedPlayer) WriteBossBarPacket(packet *bossbarpacket.BossBar) bool {
	return p.bossBarManager.WritePacket(packet)
}

// Compile-time assertion that connectedPlayer implements bossbar.ManagedViewer
var _ bossbar.ManagedViewer = (*connectedPlayer)(nil)

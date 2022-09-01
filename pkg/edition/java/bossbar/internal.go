package bossbar

import (
	"errors"
	"fmt"
	"sync"

	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

var (
	errBossBarNoUUID = errors.New("no uuid specified for boss bar")
)

type (
	barColor struct {
		color.Color
		id ColorID
	}
	bossBar struct {
		id uuid.UUID

		mu       sync.RWMutex // protects following fields
		name     component.Component
		progress float32
		color    ColorID
		viewers  map[uuid.UUID]Viewer
	}
)

func (b *bossBar) writeToViewers(p proto.Packet) {
	for _, viewer := range b.viewers {
		go func(v Viewer) { _ = v.WritePacket(p) }(viewer)
	}
}

func (b *bossBar) Name() component.Component {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.name
}
func (b *bossBar) SetName(name component.Component) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == nil || b.name == name {
		return
	}
	b.name = name
	b.writeToViewers(b.createTitleUpdate(name))
}
func (b *bossBar) Progress() float32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.progress
}
func (b *bossBar) SetProgress(progress float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if progress < MinProgress || progress > MaxProgress || b.progress == progress {
		b.mu.Unlock()
		return
	}
	b.progress = progress
	b.writeToViewers(b.createPercentUpdate(progress))
}

func (b *bossBar) createAddPacket(name component.Component) *packet.BossBar {
	return &packet.BossBar{
		UUID:   b.id,
		Action: packet.BossBarActionAdd,
		Color:  b.color,
		Flags:  b.flags,
	}
}
func (b *bossBar) createPercentUpdate(percent float32) *packet.BossBar {
	return &packet.BossBar{
		UUID:    b.id,
		Action:  packet.BossBarActionUpdatePercent,
		Percent: percent,
	}
}

func (b *bossBar) createTitleUpdate(name component.Component) *packet.BossBar {
	return &packet.BossBar{
		UUID:    b.id,
		Action:  packet.BossBarActionUpdateName,
		Name:    name,
		Color:   b.color,
		Percent: b.progress,
		Overlay: b.overlay,
		Flags:   b.flags,
	}
}

var _ BossBar = (*bossBar)(nil)

func (b *barColor) ID() ColorID { return b.id }

type bossBarManager struct {
	bars map[uuid.UUID]*BossBarHolder
	sync.Mutex

	// fnPlayer func(id uuid.UUID) *connectedPlayer
}

func (m *bossBarManager) get(bar packet.BossBar) (*BossBarHolder, bool) {
	barholder, ok := m.bars[bar.UUID]
	return barholder, ok
}

func (m *bossBarManager) getOrCreate(bar packet.BossBar) (*BossBarHolder, error) {
	if bar.UUID == uuid.Nil {
		return nil, errBossBarNoUUID
	}

	if barholder, ok := m.bars[bar.UUID]; ok {
		return barholder, nil
	}

	barholder := &BossBarHolder{
		subscribers: make(map[uuid.UUID]*proxy.connectedPlayer),
	}
	barholder.Register()

	m.bars[bar.UUID] = barholder

	return barholder, nil
}

// TODO: impl
func (m *bossBarManager) onDisconnect(player proxy.Player) {
}

func (m *bossBarManager) Add(player proxy.Player, bar packet.BossBar) error {
	m.Lock()
	defer m.Unlock()

	// this
	p, ok := player.(*proxy.connectedPlayer)
	if !ok {
		fmt.Println("Add() failed to get player")
		return nil
	}
	// or this?
	// m.fnPlayer(player.ID())

	bh, err := m.getOrCreate(bar)
	if err != nil {
		return err
	}

	bh.subscribers[player.ID()] = p
	bar.Action = packet.BossBarActionAdd
	return p.WritePacket(&bar)
}

func (m *bossBarManager) Remove(player proxy.Player, bar packet.BossBar) error {
	m.Lock()
	defer m.Unlock()

	p, ok := player.(*proxy.connectedPlayer)
	if !ok {
		fmt.Println("Remove() failed to get player")
		return nil
	}

	bh, ok := m.get(bar)
	if !ok {
		return nil
	}

	delete(bh.subscribers, player.ID())

	// delete bar when nothing is left
	if len(bh.subscribers) == 0 {
		delete(m.bars, bar.UUID)
	}

	bar.Action = packet.BossBarActionRemove
	return p.WritePacket(&bar)
}

func (m *bossBarManager) Broadcast(bar packet.BossBar) error {
	m.Lock()
	defer m.Unlock()

	bh, ok := m.get(bar)
	if !ok {
		return nil
	}

	for _, player := range bh.subscribers {
		err := player.WritePacket(&bar)
		if err != nil {
			return err
		}
	}

	return nil
}

type BossBarHolder struct {
	subscribers map[uuid.UUID]*proxy.connectedPlayer

	// register once
	sync.Once
}

// TODO: implement??
func (bbh *BossBarHolder) Register() {
}

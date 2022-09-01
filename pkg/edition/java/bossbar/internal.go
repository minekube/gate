package bossbar

import (
	"errors"
	"fmt"
	"sync"

	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy"
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
)

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

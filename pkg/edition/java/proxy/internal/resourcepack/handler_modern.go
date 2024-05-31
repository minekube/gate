package resourcepack

import (
	"bytes"
	"errors"
	"github.com/robinbraemer/event"
	"github.com/zyedidia/generic/multimap"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/util/uuid"
	"golang.org/x/exp/maps"
	"sync"
)

// modern (Minecraft 1.20.3+) resource pack handler.
type modernHandler struct {
	player   Player
	eventMgr event.Manager

	sync.RWMutex
	outstandingPacks multimap.MultiMap[uuid.UUID, *Info]
	pendingPacks     map[uuid.UUID]*Info
	appliedPacks     map[uuid.UUID]*Info
}

func newModernHandler(player Player) *modernHandler {
	return &modernHandler{
		player:           player,
		outstandingPacks: newMultiMap(),
		pendingPacks:     make(map[uuid.UUID]*Info),
		appliedPacks:     make(map[uuid.UUID]*Info),
	}
}

var newMultiMap = multimap.NewMapSlice[uuid.UUID, *Info]

var _ Handler = (*modernHandler)(nil)

func (m *modernHandler) FirstAppliedPack() *Info {
	m.RLock()
	defer m.RUnlock()
	if len(m.appliedPacks) == 0 {
		return nil
	}
	for _, info := range m.appliedPacks {
		return info
	}
	return nil
}

func (m *modernHandler) FirstPendingPack() *Info {
	m.RLock()
	defer m.RUnlock()
	if len(m.pendingPacks) == 0 {
		return nil
	}
	for _, info := range m.pendingPacks {
		return info
	}
	return nil
}

func (m *modernHandler) AppliedResourcePacks() []*Info {
	m.RLock()
	defer m.RUnlock()
	return maps.Values(m.appliedPacks)
}

func (m *modernHandler) PendingResourcePacks() []*Info {
	m.RLock()
	defer m.RUnlock()
	return maps.Values(m.pendingPacks)
}

func (m *modernHandler) ClearAppliedResourcePacks() {
	m.Lock()
	defer m.Unlock()
	m.outstandingPacks = newMultiMap()
	m.pendingPacks = make(map[uuid.UUID]*Info)
	m.appliedPacks = make(map[uuid.UUID]*Info)
}

func (m *modernHandler) Remove(id uuid.UUID) bool {
	m.Lock()
	defer m.Unlock()
	m.outstandingPacks.RemoveAll(id)
	_, ok := m.appliedPacks[id]
	_, ok2 := m.pendingPacks[id]
	delete(m.appliedPacks, id)
	delete(m.pendingPacks, id)
	return ok || ok2
}

func (m *modernHandler) QueueResourcePack(info *Info) error {
	m.Lock()
	m.outstandingPacks.Put(info.ID, info)
	if m.outstandingPacks.Count(info.ID) == 1 {
		id := m.outstandingPacks.Get(info.ID)[0].ID
		m.Unlock()
		return m.tickResourcePackQueue(id)
	}
	m.Unlock()
	return nil
}

func (m *modernHandler) QueueResourcePackRequest(request *packet.ResourcePackRequest) error {
	// only a single pack in request for now, not a bundle
	return queueResourcePackRequest(request, m)
}

func (m *modernHandler) CheckAlreadyAppliedPack(hash []byte) error {
	return checkAlreadyAppliedPack(hash, m)
}

func (m *modernHandler) tickResourcePackQueue(id uuid.UUID) error {
	m.RLock()
	outstandingResourcePacks := m.outstandingPacks.Get(id)
	if len(outstandingResourcePacks) != 0 {
		pack := m.outstandingPacks.Get(id)[0]
		m.RUnlock()
		return m.SendResourcePackRequestPacket(pack)
	}
	m.RUnlock()
	return nil
}

func (m *modernHandler) OnResourcePackResponse(bundle *ResponseBundle) (bool, error) {
	id := bundle.ID

	m.Lock()
	defer m.Unlock()

	outstandingResourcePacks := m.outstandingPacks.Get(id)
	peek := bundle.Status.Intermediate()
	var queued *Info
	if len(outstandingResourcePacks) != 0 {
		if peek {
			queued = outstandingResourcePacks[0]
		} else {
			queued = outstandingResourcePacks[0]
			m.outstandingPacks.Remove(id, queued)
		}
	}

	e := newPlayerResourcePackStatusEvent(m.player, bundle.Status, queued.ID, *queued)
	event.FireParallel(m.eventMgr, e, func(e *PlayerResourcePackStatusEvent) {
		if e.Status() == packet.DeclinedResourcePackResponseStatus &&
			e.PackInfo().ShouldForce &&
			!e.OverwriteKick() {
			m.player.Disconnect(&component.Translation{
				Key: "multiplayer.requiredTexturePrompt.disconnect",
			})
		}
	})

	switch bundle.Status {
	// The player has accepted the resource pack and will proceed to download it.
	case packet.AcceptedResourcePackResponseStatus:
		if queued != nil {
			m.pendingPacks[id] = queued
		}
	// The resource pack has been applied correctly.
	case packet.SuccessfulResourcePackResponseStatus:
		delete(m.pendingPacks, id)
		if queued != nil {
			m.appliedPacks[id] = queued
		} else {
			// When transitioning to another server that has a resource pack to apply,
			// if one or more resource packs have already been applied from Velocity,
			// the player sends more than 1 SUCCESSFUL response to the backend server,
			// which results in the server receiving more resource pack responses
			// than the server has sent requests to the player
			appliedPack, ok := m.appliedPacks[id]
			if ok {
				return m.HandleResponseResult(appliedPack, bundle)
			}
		}
	// An error occurred while trying to download the resource pack to the client,
	// so the resource pack cannot be applied.
	case packet.DiscardedResourcePackResponseStatus:
		delete(m.pendingPacks, id)
		delete(m.appliedPacks, id)
	// The other cases in which no action is taken are documented in the javadocs.
	default:
	}

	var err error
	if !peek {
		err = m.tickResourcePackQueue(id)
	}
	handled, err2 := m.HandleResponseResult(queued, bundle)
	return handled, errors.Join(err, err2)
}

func (m *modernHandler) HasPackAppliedByHash(hash []byte) bool {
	m.RLock()
	defer m.RUnlock()
	for _, info := range m.appliedPacks {
		if bytes.Equal(info.Hash, hash) {
			return true
		}
	}
	return false
}

func (m *modernHandler) SendResourcePackRequestPacket(pack *Info) error {
	return sendResourcePackRequestPacket(pack, m.player)
}

func (m *modernHandler) HandleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool, err error) {
	return handleResponseResult(queued, bundle, m.player)
}

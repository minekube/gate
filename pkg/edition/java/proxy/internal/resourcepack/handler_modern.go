package resourcepack

import (
	"github.com/zyedidia/generic/multimap"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/util/uuid"
	"sync"
)

// modern (Minecraft 1.20.3+) resource pack handler.
type modernHandler struct {
	baseHandler

	sync.RWMutex
	outstandingPacks multimap.MultiMap[uuid.UUID, *Info]
	pendingPacks     map[uuid.UUID]*Info
	appliedPacks     map[uuid.UUID]*Info
}

func newModernHandler(player Player) *modernHandler {
	h := &modernHandler{
		baseHandler: baseHandler{
			player: player,
		},
		outstandingPacks: newMultiMap(),
		pendingPacks:     make(map[uuid.UUID]*Info),
		appliedPacks:     make(map[uuid.UUID]*Info),
	}
	h.baseHandler.parent = h
	return h
}

var newMultiMap = multimap.NewMapSlice[uuid.UUID, *Info]

var _ HandlerInterface = (*modernHandler)(nil)

func (m *modernHandler) FirstAppliedPack() *Info {
	m.RLock()
	defer m.RUnlock()
	if len(m.applied) == 0 {
		return nil
	}
	return m.applied[0]
}

func (m *modernHandler) FirstPendingPack() *Info {
	m.RLock()
	defer m.RUnlock()
	if len(m.pending) == 0 {
		return nil
	}
	return m.pending[0]
}

func (m *modernHandler) AppliedResourcePacks() []*Info {
	m.RLock()
	defer m.RUnlock()
	return m.applied
}

func (m *modernHandler) PendingResourcePacks() []*Info {
	m.RLock()
	defer m.RUnlock()
	return m.pending
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
		return m.tickResourcePackQueue(m.outstandingPacks.Get(info.ID)[0].ID)
	}
	m.Unlock()
	return nil
}

func (m *modernHandler) QueueResourcePackRequest(request *packet.ResourcePackRequest) error {

}

func (m *modernHandler) OnResourcePackResponse(bundle *ResponseBundle) {
	//TODO implement me
	panic("implement me")
}

func (m *modernHandler) HasPackAppliedByHash(hash []byte) bool {
	//TODO implement me
	panic("implement me")
}

func (m *modernHandler) CheckAlreadyAppliedPack(hash []byte) error {
	//TODO implement me
	panic("implement me")
}

package tablist

import (
	"fmt"
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type legacyTabList struct {
	*tabList

	mu          sync.RWMutex
	nameMapping map[string]uuid.UUID
}

// NewLegacy returns a new legacy TabList for version <= 1.7.
func NewLegacy(w PacketWriter, p proto.Protocol) TabList {
	return &legacyTabList{
		tabList:     newTabList(w, p, &nopKeyStore{}),
		nameMapping: map[string]uuid.UUID{},
	}
}

// SetHeaderFooter is not available in legacy.
func (t *legacyTabList) SetHeaderFooter(_, _ component.Component) error { return nil }

// ClearHeaderFooter is not available in legacy.
func (t *legacyTabList) ClearHeaderFooter() error { return nil }

func (t *legacyTabList) AddEntry(entry Entry) error {
	err := t.tabList.AddEntry(entry)
	if err != nil {
		t.mu.Lock()
		t.nameMapping[entry.Profile().Name] = entry.Profile().ID
		t.mu.Unlock()
	}
	return err
}

func (t *legacyTabList) RemoveEntry(id uuid.UUID) error {
	entry, err := t.tabList.removeEntry(id)
	if entry != nil {
		t.mu.Lock()
		delete(t.nameMapping, entry.Profile().Name)
		t.mu.Unlock()
	}
	return err
}

func (t *legacyTabList) HasEntry(id uuid.UUID) bool {
	return t.tabList.HasEntry(id)
}

func (t *legacyTabList) Entries() map[uuid.UUID]Entry {
	return t.tabList.Entries()
}

func (t *legacyTabList) ProcessBackendPacket(p *packet.PlayerListItem) error {
	if len(p.Items) != 1 {
		return fmt.Errorf("expected 1 item in %T but got %d", p, len(p.Items))
	}
	item := p.Items[0] // Only one item per packet in 1.7

	switch p.Action {
	case packet.AddPlayerListItemAction:
		t.mu.Lock()
		if id, ok := t.nameMapping[item.Name]; ok { // ADD_PLAYER also used for updating ping
			t.mu.Unlock()
			if entry := t.tabList.entry(id); entry != nil {
				entry.setLatency(time.Millisecond * time.Duration(item.Latency))
			}
		} else {
			id := uuid.New() // Use a fake uuid to preserve function of custom entries
			t.nameMapping[item.Name] = id
			t.mu.Unlock()
			return t.AddEntry(&tabListEntry{
				tabList:          t,
				onSetDisplayName: func() { _ = t.RemoveEntry(id) },
				profile: &profile.GameProfile{
					ID:   id,
					Name: item.Name,
				},
				latency: time.Millisecond * time.Duration(item.Latency),
			})
		}
	case packet.RemovePlayerListItemAction:
		t.mu.Lock()
		removedID := t.nameMapping[item.Name]
		t.mu.Unlock()
		if removedID != uuid.Nil {
			return t.tabList.RemoveEntry(removedID)
		}
	default:
		// For 1.7 there is only add and remove
	}
	return nil
}

func (t *legacyTabList) updateEntry(action packet.PlayerListItemAction, entry *tabListEntry) error {
	if !t.HasEntry(entry.Profile().ID) {
		return nil
	}
	switch action {
	case packet.UpdateLatencyPlayerListItemAction, packet.UpdateDisplayNamePlayerListItemAction:
		return t.tabList.w.WritePacket(&packet.PlayerListItem{
			Action: packet.AddPlayerListItemAction,
			Items:  []packet.PlayerListItemEntry{*newPlayerListItemEntry(entry)},
		})
	default:
		// Can't do anything else
	}
	return nil
}

type nopKeyStore struct{}

func (*nopKeyStore) PlayerKey(uuid.UUID) crypto.IdentifiedKey { return nil }

var _ PlayerKey = (*nopKeyStore)(nil)

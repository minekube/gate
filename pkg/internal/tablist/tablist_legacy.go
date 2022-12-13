package tablist

import (
	"fmt"
	"sync"
	"time"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/util/uuid"
)

type LegacyTabList struct {
	KeyedTabList
	NameMapping map[string]uuid.UUID
	sync.Mutex
}

var _ tablist.TabList = (*LegacyTabList)(nil)

func (l *LegacyTabList) Add(entries ...tablist.Entry) error {
	l.Lock()
	defer l.Unlock()
	for _, entry := range entries {
		l.NameMapping[entry.Profile().Name] = entry.Profile().ID
	}
	return l.KeyedTabList.Add(entries...)
}

func (l *LegacyTabList) RemoveAll(ids ...uuid.UUID) error {
	l.Lock()
	defer l.Unlock()
	for _, id := range ids {
		for name, id2 := range l.NameMapping {
			if id == id2 {
				delete(l.NameMapping, name)
			}
		}
	}
	return l.KeyedTabList.RemoveAll(ids...)
}

func (l *LegacyTabList) ProcessLegacy(p *legacytablist.PlayerListItem) error {
	if len(p.Items) == 0 {
		return fmt.Errorf("expected at least one item in %T but got zero", p)
	}
	item := p.Items[0] // Only one item per packet in 1.7

	l.Lock()
	defer l.Unlock()

	l.KeyedTabList.Lock()

	switch p.Action {
	case legacytablist.AddPlayerListItemAction:
		if id, ok := l.NameMapping[item.Name]; ok { // ADD_PLAYER also used for updating ping
			if entry := l.EntriesByID[id]; entry != nil {
				doInternalEntity(entry, func(e internalEntry) {
					e.SetLatencyInternal(time.Millisecond * time.Duration(item.Latency))
				})
			}
		} else {
			id := uuid.New() // Use a fake uuid to preserve function of custom entries
			l.NameMapping[item.Name] = id

			l.KeyedTabList.Unlock()
			return l.KeyedTabList.Add(&KeyedEntry{
				Entry: Entry{
					OwningTabList: ResolveRoot(l),
					EntryAttributes: EntryAttributes{
						Latency: time.Millisecond * time.Duration(item.Latency),
						Profile: profile.GameProfile{
							ID:   id,
							Name: item.Name,
						},
					},
				},
			})
		}
	case legacytablist.RemovePlayerListItemAction:
		removedID, ok := l.NameMapping[item.Name]
		if ok {
			delete(l.EntriesByID, removedID)
		}
	default:
		// For 1.7 there is only add and remove
	}

	l.KeyedTabList.Unlock()
	return nil
}

func (l *LegacyTabList) UpdateEntry(action legacytablist.PlayerListItemAction, entry tablist.Entry) error {
	if !l.hasEntry(entry.Profile().ID) {
		return nil
	}

	switch action {
	case legacytablist.UpdateLatencyPlayerListItemAction,
		legacytablist.UpdateDisplayNamePlayerListItemAction: // Add here because we removed beforehand

		return l.Viewer.WritePacket(&legacytablist.PlayerListItem{
			Action: legacytablist.AddPlayerListItemAction, // ADD_PLAYER also updates ping
			Items:  []legacytablist.PlayerListItemEntry{*toLegacyEntry(entry)},
		})
	}

	return nil
}

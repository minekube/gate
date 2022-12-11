package tablist

import (
	"fmt"
	"sync"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/util/uuid"
)

type InternalTabList interface {
	tablist.TabList

	ProcessRemove(info *playerinfo.Remove)
	ProcessUpdate(info *playerinfo.Upsert) error
	ProcessLegacy(legacy *legacytablist.PlayerListItem) error

	EmitActionRaw(action playerinfo.UpsertAction, entry *playerinfo.Entry) error
	UpdateEntry(action legacytablist.PlayerListItemAction, entry tablist.Entry) error
}

type (
	TabList struct {
		Viewer tablist.Viewer

		sync.RWMutex
		EntriesByID map[uuid.UUID]tablist.Entry
	}
)

var _ InternalTabList = (*TabList)(nil)

func (t *TabList) RemoveAll(ids ...uuid.UUID) error {
	toRemove := t.deleteEntries(ids...)
	return t.Viewer.WritePacket(&playerinfo.Remove{
		PlayersToRemove: toRemove,
	})
}

func (t *TabList) deleteEntries(ids ...uuid.UUID) []uuid.UUID {
	t.Lock()
	defer t.Unlock()
	if len(ids) == 0 { // Delete all
		ids = make([]uuid.UUID, 0, len(t.EntriesByID))
		for id := range t.EntriesByID {
			ids = append(ids, id)
		}
		t.EntriesByID = make(map[uuid.UUID]tablist.Entry)
		return ids
	}
	for _, id := range ids {
		delete(t.EntriesByID, id)
	}
	return ids
}

func (t *TabList) Entries() map[uuid.UUID]tablist.Entry {
	t.RLock()
	defer t.RUnlock()
	// Copy
	entries := make(map[uuid.UUID]tablist.Entry, len(t.EntriesByID))
	for id, entry := range t.EntriesByID {
		entries[id] = entry
	}
	return entries
}

func (t *TabList) Add(entries ...tablist.Entry) error {
	for _, entry := range entries {
		if err := t.add(entry); err != nil {
			return fmt.Errorf("error adding tab list entry %s: %w", entry.Profile(), err)
		}
	}
	return nil
}

func (t *TabList) add(entry tablist.Entry) error {
	if entry.Profile().ID == uuid.Nil {
		return fmt.Errorf("profile id must not be zero")
	}

	var actions []playerinfo.UpsertAction
	playerInfoEntry := &playerinfo.Entry{
		ProfileID: entry.Profile().ID,
	}

	t.Lock()
	previousEntry := t.EntriesByID[playerInfoEntry.ProfileID]
	t.EntriesByID[playerInfoEntry.ProfileID] = entry
	t.Unlock()
}

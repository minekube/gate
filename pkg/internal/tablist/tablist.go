package tablist

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// New creates a new tab list for the given viewer.
func New(viewer Viewer) InternalTabList {
	if viewer.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
		return &TabList{
			ParentStruct: nil,
			Viewer:       viewer,
			EntriesByID:  map[uuid.UUID]tablist.Entry{},
		}
	}
	if viewer.Protocol().GreaterEqual(version.Minecraft_1_8) {
		tl := &KeyedTabList{}
		tl.TabList = TabList{
			ParentStruct: tl,
			Viewer:       viewer,
			EntriesByID:  map[uuid.UUID]tablist.Entry{},
		}
		return tl
	}

	tl := &LegacyTabList{
		NameMapping: map[string]uuid.UUID{},
	}
	tl.TabList = TabList{
		ParentStruct: tl,
		Viewer:       viewer,
		EntriesByID:  map[uuid.UUID]tablist.Entry{},
	}
	return tl
}

type InternalTabList interface {
	tablist.TabList

	GetViewer() tablist.Viewer

	ProcessRemove(info *playerinfo.Remove)
	ProcessUpdate(info *playerinfo.Upsert) error
	ProcessLegacy(legacy *legacytablist.PlayerListItem) error

	EmitActionRaw(action playerinfo.UpsertAction, entry *playerinfo.Entry) error
	UpdateEntry(action legacytablist.PlayerListItemAction, entry tablist.Entry) error

	Parent() InternalTabList // Used to resolve the parent root struct of an embedded tab list struct
}

// ResolveRoot returns the root structure of the tab list.
func ResolveRoot(i InternalTabList) InternalTabList {
	for {
		if i.Parent() == nil {
			return i
		}
		i = i.Parent()
	}
}

type Viewer interface {
	tablist.Viewer

	BufferPacket(packet proto.Packet) (err error)
	Flush() error
}

type (
	TabList struct {
		ParentStruct InternalTabList
		Viewer       Viewer

		sync.RWMutex
		EntriesByID map[uuid.UUID]tablist.Entry

		headerFooter struct {
			sync.RWMutex
			header, footer component.Component
		}
	}
)

func (t *TabList) GetViewer() tablist.Viewer {
	return t.Viewer
}

func (t *TabList) SetHeaderFooter(header, footer component.Component) error {
	if header == nil {
		header = &component.Translation{}
	}
	if footer == nil {
		footer = &component.Translation{}
	}

	err := tablist.SendHeaderFooter(t.Viewer, header, footer)
	if err != nil {
		return fmt.Errorf("error sending header footer: %w", err)
	}

	t.headerFooter.Lock()
	t.headerFooter.header = header
	t.headerFooter.footer = footer
	t.headerFooter.Unlock()

	return nil
}

func (t *TabList) HeaderFooter() (header, footer component.Component) {
	t.headerFooter.RLock()
	defer t.headerFooter.RUnlock()
	return t.headerFooter.header, t.headerFooter.footer
}

func (t *TabList) Parent() InternalTabList {
	return t.ParentStruct
}

var _ InternalTabList = (*TabList)(nil)

func (t *TabList) EmitActionRaw(action playerinfo.UpsertAction, entry *playerinfo.Entry) error {
	return t.Viewer.WritePacket(&playerinfo.Upsert{
		ActionSet: []playerinfo.UpsertAction{action},
		Entries:   []*playerinfo.Entry{entry},
	})
}

func (t *TabList) UpdateEntry(action legacytablist.PlayerListItemAction, entry tablist.Entry) error {
	return nil
}

func (t *TabList) RemoveAll(ids ...uuid.UUID) error {
	if toRemove := t.deleteEntries(ids...); len(toRemove) != 0 {
		return t.Viewer.BufferPacket(&playerinfo.Remove{
			PlayersToRemove: toRemove,
		})
	}
	return nil
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
	if len(entries) == 0 {
		return nil
	}
	var flush bool
	for _, entry := range entries {
		pkt, err := t.add(entry)
		if err != nil {
			return fmt.Errorf("error adding tab list entry %s: %w", entry.Profile(), err)
		}
		if len(pkt.ActionSet) == 0 {
			continue
		}
		err = t.Viewer.BufferPacket(pkt)
		if err != nil {
			return fmt.Errorf("error buffering tab list entry %s: %w", entry.Profile(), err)
		}
		flush = true
	}
	if flush {
		return t.Viewer.Flush()
	}
	return nil
}

func (t *TabList) add(entry tablist.Entry) (*playerinfo.Upsert, error) {
	if entry.Profile().ID == uuid.Nil {
		return nil, fmt.Errorf("profile id must not be zero")
	}

	var actions []playerinfo.UpsertAction
	playerInfoEntry := &playerinfo.Entry{
		ProfileID: entry.Profile().ID,
		GameMode:  entry.GameMode(),
		Listed:    entry.Listed(),
		ListOrder: entry.ListOrder(),
	}

	t.Lock()
	previousEntry := t.EntriesByID[playerInfoEntry.ProfileID]
	t.EntriesByID[playerInfoEntry.ProfileID] = entry
	t.Unlock()

	if previousEntry != nil {
		// we should merge entries here
		if equalLocked(previousEntry, entry) {
			return nil, nil // nothing else to do, this entry is perfect
		}
		if !reflect.DeepEqual(previousEntry.DisplayName(), entry.DisplayName()) {
			actions = append(actions, playerinfo.UpdateDisplayNameAction)
			playerInfoEntry.DisplayName = chat.FromComponentProtocol(entry.DisplayName(), t.Viewer.Protocol())
		}
		if previousEntry.Latency() != entry.Latency() {
			actions = append(actions, playerinfo.UpdateLatencyAction)
			playerInfoEntry.Latency = int(entry.Latency().Milliseconds())
		}
		if previousEntry.GameMode() != entry.GameMode() {
			actions = append(actions, playerinfo.UpdateGameModeAction)
			playerInfoEntry.GameMode = entry.GameMode()
		}
		if previousEntry.Listed() != entry.Listed() {
			actions = append(actions, playerinfo.UpdateListedAction)
			playerInfoEntry.Listed = entry.Listed()
		}
		if previousEntry.ListOrder() != entry.ListOrder() && t.Viewer.Protocol().GreaterEqual(version.Minecraft_1_21_2) {
			actions = append(actions, playerinfo.UpdateListOrderAction)
			playerInfoEntry.ListOrder = entry.ListOrder()
		}
		if !reflect.DeepEqual(previousEntry.ChatSession(), entry.ChatSession()) {
			if from := entry.ChatSession(); from != nil {
				actions = append(actions, playerinfo.InitializeChatAction)
				playerInfoEntry.RemoteChatSession = &chat.RemoteChatSession{
					ID:  from.SessionID(),
					Key: from.IdentifiedKey(),
				}
			}
		}
	} else {
		actions = append(actions,
			playerinfo.AddPlayerAction,
			playerinfo.UpdateLatencyAction,
			playerinfo.UpdateListedAction,
		)
		playerInfoEntry.Profile = entry.Profile()
		if entry.DisplayName() != nil {
			actions = append(actions, playerinfo.UpdateDisplayNameAction)
			playerInfoEntry.DisplayName = chat.FromComponentProtocol(entry.DisplayName(), t.Viewer.Protocol())
		}
		if entry.ChatSession() != nil {
			actions = append(actions, playerinfo.InitializeChatAction)
			playerInfoEntry.RemoteChatSession = &chat.RemoteChatSession{
				ID:  entry.ChatSession().SessionID(),
				Key: entry.ChatSession().IdentifiedKey(),
			}
		}
		if entry.GameMode() != -1 && entry.GameMode() != 256 {
			actions = append(actions, playerinfo.UpdateGameModeAction)
			playerInfoEntry.GameMode = entry.GameMode()
		}
		playerInfoEntry.Latency = int(entry.Latency().Milliseconds())
		playerInfoEntry.Listed = entry.Listed()
		if entry.ListOrder() != 0 && t.Viewer.Protocol().GreaterEqual(version.Minecraft_1_21_2) {
			actions = append(actions, playerinfo.UpdateListOrderAction)
			playerInfoEntry.ListOrder = entry.ListOrder()
		}
	}

	return &playerinfo.Upsert{
		ActionSet: actions,
		Entries: []*playerinfo.Entry{
			playerInfoEntry,
		},
	}, nil
}

func (t *TabList) hasEntry(id uuid.UUID) bool {
	if t.TryRLock() {
		defer t.RUnlock()
	}
	_, ok := t.EntriesByID[id]
	return ok
}

func (t *TabList) ProcessLegacy(legacy *legacytablist.PlayerListItem) error {
	return nil
}

func (t *TabList) ProcessRemove(info *playerinfo.Remove) {
	t.Lock()
	for _, entry := range info.PlayersToRemove {
		delete(t.EntriesByID, entry)
	}
	t.Unlock()
}

func (t *TabList) ProcessUpdate(info *playerinfo.Upsert) error {
	t.Lock()
	defer t.Unlock()
	for _, entry := range info.Entries {
		err := t.processUpdateForEntry(info.ActionSet, entry)
		if err != nil {
			return fmt.Errorf("error processing tab list update for %s: %w", entry.ProfileID, err)
		}
	}
	return nil
}

func (t *TabList) processUpdateForEntry(actions []playerinfo.UpsertAction, info *playerinfo.Entry) error {
	profileID := info.ProfileID
	currentEntry := t.EntriesByID[profileID]
	if playerinfo.ContainsAction(actions, playerinfo.AddPlayerAction) {
		if currentEntry == nil {
			currentEntry = &Entry{
				OwningTabList: ResolveRoot(t),
				EntryAttributes: EntryAttributes{
					Profile:  info.Profile,
					GameMode: -1,
				},
			}
			t.EntriesByID[profileID] = currentEntry
		} // else: Received an add player packet for an existing entry; this does nothing.
	} else if currentEntry == nil {
		// Received a partial player before an ADD_PLAYER action; profile could not be built.
		return nil
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateGameModeAction) {
		doInternalEntity(currentEntry, func(e internalEntry) {
			e.SetGameModeInternal(info.GameMode)
		})
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateLatencyAction) {
		doInternalEntity(currentEntry, func(e internalEntry) {
			e.SetLatencyInternal(time.Duration(info.Latency) * time.Millisecond)
		})
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateDisplayNameAction) {
		doInternalEntity(currentEntry, func(e internalEntry) {
			e.SetDisplayNameInternal(info.DisplayName.AsComponentOrNil())
		})
	}
	if playerinfo.ContainsAction(actions, playerinfo.InitializeChatAction) {
		doInternalEntity(currentEntry, func(e internalEntry) {
			e.SetChatSessionInternal(info.RemoteChatSession)
		})
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateListedAction) {
		doInternalEntity(currentEntry, func(e internalEntry) {
			e.SetListedInternal(info.Listed)
		})
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateListOrderAction) {
		doInternalEntity(currentEntry, func(e internalEntry) {
			e.SetListOrderInternal(info.ListOrder)
		})
	}
	return nil
}

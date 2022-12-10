package tablist

import (
	"fmt"
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/util/uuid"
)

type (
	TabList struct {
		Viewer tablist.Viewer

		sync.RWMutex
		Entries map[uuid.UUID]*Entry
	}
)

func (t *TabList) ProcessRemove(info *playerinfo.Remove) {
	t.Lock()
	for _, entry := range info.PlayersToRemove {
		delete(t.Entries, entry)
	}
	t.Unlock()
}

func (t *TabList) ProcessUpdate(info *playerinfo.Upsert) error {
	t.Lock()
	defer t.Unlock()
	for _, entry := range info.Entries {
		err := t.ProcessUpdateForEntry(info.ActionSet, entry)
		if err != nil {
			return fmt.Errorf("error processing tab list update for %s: %w", entry.ProfileID, err)
		}
	}
	return nil
}

func (t *TabList) ProcessUpdateForEntry(actions []playerinfo.UpsertAction, info *playerinfo.Entry) error {
	if info.ProfileID == uuid.Nil {
		return fmt.Errorf("profile id must not be nil")
	}
	profileID := info.ProfileID
	currentEntry := t.Entries[profileID]
	if playerinfo.ContainsAction(actions, playerinfo.AddPlayerAction) {
		if currentEntry == nil {
			currentEntry = &Entry{
				Profile:  info.Profile,
				GameMode: -1,
			}
			t.Entries[profileID] = currentEntry
		} // else: Received an add player packet for an existing entry; this does nothing.
	} else if currentEntry == nil {
		// Received a partial player before an ADD_PLAYER action; profile could not be built.
		return nil
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateGameModeAction) {
		currentEntry.GameMode = info.GameMode
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateLatencyAction) {
		currentEntry.Latency = time.Millisecond * time.Duration(info.Latency)
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateDisplayNameAction) {
		currentEntry.DisplayName = info.DisplayName
	}
	if playerinfo.ContainsAction(actions, playerinfo.InitializeChatAction) {
		currentEntry.ChatSession = info.RemoteChatSession
	}
	if playerinfo.ContainsAction(actions, playerinfo.UpdateListedAction) {
		currentEntry.Listed = info.Listed
	}
	return nil
}

type Entry struct {
	Profile     *profile.GameProfile
	DisplayName component.Component // nil-able
	Latency     time.Duration
	GameMode    int
	Listed      bool
	ChatSession *chat.RemoteChatSession
}

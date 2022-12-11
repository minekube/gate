package tablist

import (
	"fmt"
	"time"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/util/uuid"
)

func (t *TabList) ProcessLegacy(legacy *legacytablist.PlayerListItem) error {
	//TODO implement me
	panic("implement me")
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
	if info.ProfileID == uuid.Nil {
		return fmt.Errorf("profile id must not be nil")
	}
	profileID := info.ProfileID
	currentEntry := t.EntriesByID[profileID]
	if playerinfo.ContainsAction(actions, playerinfo.AddPlayerAction) {
		if currentEntry == nil {
			currentEntry = &Entry{
				Profile:  info.Profile,
				GameMode: -1,
			}
			t.EntriesByID[profileID] = currentEntry
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

package tablist

import (
	"errors"
	"fmt"
	"time"

	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/util/uuid"
)

type KeyedTabList struct {
	TabList
}

var _ InternalTabList = (*KeyedTabList)(nil)

func (k *KeyedTabList) Parent() InternalTabList { return nil }
func (k *KeyedTabList) Add(entries ...tablist.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	err := func() error {
		k.TabList.Lock()
		defer k.TabList.Unlock()

		for i, entry := range entries {
			if entry == nil {
				return fmt.Errorf("entry at index %d is nil", i)
			}
			if entry.TabList() != k {
				return fmt.Errorf("entry %s at index %d is not from this tab list", entry.Profile(), i)
			}
			if k.TabList.hasEntry(entry.Profile().ID) {
				return fmt.Errorf("entry %s already exists with same id", entry.Profile())
			}

			pkt := &legacytablist.PlayerListItem{
				Action: legacytablist.AddPlayerListItemAction,
				Items:  []legacytablist.PlayerListItemEntry{*toLegacyEntry(entry)},
			}
			err := k.TabList.Viewer.BufferPacket(pkt)
			if err != nil {
				return fmt.Errorf("error writing %T packet: %w", pkt, err)
			}

			k.TabList.EntriesByID[entry.Profile().ID] = entry
		}

		return nil
	}()
	if err != nil {
		return err
	}

	return k.TabList.Viewer.Flush()
}

func (k *KeyedTabList) RemoveAll(ids ...uuid.UUID) error {
	toRemove := k.TabList.DeleteEntries(ids...)
	items := make([]legacytablist.PlayerListItemEntry, 0, len(toRemove))
	for _, id := range toRemove {
		items = append(items, legacytablist.PlayerListItemEntry{
			ID: id,
		})
	}
	return k.TabList.Viewer.BufferPacket(&legacytablist.PlayerListItem{
		Action: legacytablist.RemovePlayerListItemAction,
		Items:  items,
	})
}

func (k *KeyedTabList) ProcessRemove(info *playerinfo.Remove) {}

func (k *KeyedTabList) ProcessUpdate(info *playerinfo.Upsert) error {
	return nil
}

func (k *KeyedTabList) ProcessLegacy(p *legacytablist.PlayerListItem) error {
	// Packet is already forwarded on, so no need to do that here
	k.TabList.Lock()
	defer k.TabList.Unlock()
	for _, item := range p.Items {
		if item.ID == uuid.Nil {
			return errors.New("1.7 tab list entry given to modern tab list handler")
		}

		if p.Action != legacytablist.AddPlayerListItemAction && !k.TabList.hasEntry(item.ID) {
			// Sometimes UpdateGameMode is sent before AddPlayer so don't want to warn here
			continue
		}

		switch p.Action {
		case legacytablist.AddPlayerListItemAction:
			// ensure that name and properties are available
			if item.Name == "" || item.Properties == nil {
				return errors.New("got null game profile for AddPlayerListItemAction")
			}

			/* why are we verifying the key here - multi-proxy setups break this
			// Verify key
			providedKey := item.PlayerKey
			if expectedKey := t.keyProvider.PlayerKey(item.ID); expectedKey != nil {
				if providedKey == nil {
					// Substitute the key
					// It shouldn't be propagated to remove the signature.
					providedKey = expectedKey
				} else {
					if !crypto.Equal(expectedKey, providedKey) {
						return fmt.Errorf("server provided incorrect player key in playerlist"+
							" for player %s UUID: %s", item.Name, item.ID)
					}
				}
			}
			*/

			if !k.TabList.hasEntry(item.ID) {
				k.TabList.EntriesByID[item.ID] = &KeyedEntry{
					Entry: Entry{
						OwningTabList: ResolveRoot(k),
						EntryAttributes: EntryAttributes{
							Profile: profile.GameProfile{
								ID:         item.ID,
								Name:       item.Name,
								Properties: item.Properties,
							},
							DisplayName: item.DisplayName,
							Latency:     time.Duration(item.Latency) * time.Millisecond,
							GameMode:    item.GameMode,
							ChatSession: &chat.RemoteChatSession{
								ID:  uuid.Nil,
								Key: item.PlayerKey,
							},
						},
					},
				}
			}
		case legacytablist.RemovePlayerListItemAction:
			delete(k.TabList.EntriesByID, item.ID)
		case legacytablist.UpdateDisplayNamePlayerListItemAction:
			e, ok := k.TabList.EntriesByID[item.ID]
			if ok {
				doInternalEntity(e, func(e internalEntry) {
					e.SetDisplayNameInternal(item.DisplayName)
				})
			}
		case legacytablist.UpdateLatencyPlayerListItemAction:
			e, ok := k.TabList.EntriesByID[item.ID]
			if ok {
				doInternalEntity(e, func(e internalEntry) {
					e.SetLatencyInternal(time.Duration(item.Latency) * time.Millisecond)
				})
			}
		case legacytablist.UpdateGameModePlayerListItemAction:
			e, ok := k.TabList.EntriesByID[item.ID]
			if ok {
				doInternalEntity(e, func(e internalEntry) {
					e.SetGameModeInternal(item.GameMode)
				})
			}
		default:
			// Nothing we can do here
		}
	}
	return nil
}

func (k *KeyedTabList) EmitActionRaw(action playerinfo.UpsertAction, entry *playerinfo.Entry) error {
	return nil
}

func (k *KeyedTabList) UpdateEntry(action legacytablist.PlayerListItemAction, entry tablist.Entry) error {
	k.TabList.Lock()
	defer k.TabList.Unlock()

	profileID := entry.Profile().ID
	if !k.TabList.hasEntry(profileID) {
		return nil
	}

	item := toLegacyEntry(entry)
	selectedKey := k.TabList.Viewer.IdentifiedKey()
	if selectedKey != nil &&
		keyrevision.Applicable(selectedKey.KeyRevision(), k.TabList.Viewer.Protocol()) &&
		selectedKey.SignatureHolder() == profileID {
		item.PlayerKey = selectedKey
	} else {
		item.PlayerKey = nil
	}

	return k.TabList.Viewer.WritePacket(&legacytablist.PlayerListItem{
		Action: action,
		Items:  []legacytablist.PlayerListItemEntry{*item},
	})
}

func toLegacyEntry(entry tablist.Entry) *legacytablist.PlayerListItemEntry {
	p := entry.Profile()
	return &legacytablist.PlayerListItemEntry{
		ID:          p.ID,
		Name:        p.Name,
		DisplayName: entry.DisplayName(),
		Latency:     int(entry.Latency().Milliseconds()),
		GameMode:    entry.GameMode(),
		Properties:  p.Properties,
		PlayerKey:   entry.ChatSession().IdentifiedKey(),
	}
}

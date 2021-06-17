package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	util2 "go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/util/uuid"
	"sync"
	"time"
)

type tabList struct {
	c *minecraftConn

	mu      sync.RWMutex
	entries map[uuid.UUID]*tabListEntry
}

var _ player.TabList = (*tabList)(nil)

func newTabList(c *minecraftConn) *tabList {
	return &tabList{c: c, entries: map[uuid.UUID]*tabListEntry{}}
}

func (t *tabList) AddEntry(entry player.TabListEntry) error {
	if entry == nil {
		return errors.New("entry must not be nil")
	}
	e, ok := entry.(*tabListEntry)
	if !ok {
		return errors.New("entry must not be an external implementation")
	}
	if entry.TabList() != t {
		return errors.New("provided entry must be created by the tab list")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hasEntry(entry.Profile().ID) {
		return errors.New("tab list already has entry of same profile id")
	}
	t.entries[entry.Profile().ID] = e
	return t.c.WritePacket(&packet.PlayerListItem{
		Action: packet.AddPlayerListItemAction,
		Items:  []packet.PlayerListItemEntry{*newPlayerListItemEntry(entry)},
	})
}

func (t *tabList) RemoveEntry(id uuid.UUID) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.entries[id]
	if !ok {
		// Ignore if not found
		return nil
	}
	delete(t.entries, id)
	return t.c.WritePacket(&packet.PlayerListItem{
		Action: packet.RemovePlayerListItemAction,
		Items:  []packet.PlayerListItemEntry{*newPlayerListItemEntry(entry)},
	})
}

func (t *tabList) SetHeaderFooter(header, footer component.Component) error {
	b := new(bytes.Buffer)
	p := new(packet.HeaderAndFooter)
	j := util2.JsonCodec(t.c.protocol)

	if err := j.Marshal(b, header); err != nil {
		return fmt.Errorf("error marshal header: %w", err)
	}
	p.Header = b.String()
	b.Reset()
	if err := j.Marshal(b, footer); err != nil {
		return fmt.Errorf("error marshal footer: %w", err)
	}
	p.Footer = b.String()

	return t.c.WritePacket(p)
}

func (t *tabList) ClearHeaderFooter() error {
	return t.c.WritePacket(packet.ResetHeaderAndFooter)
}

// removes all player entries shown in the tab list
func (t *tabList) clearEntries() error {
	items, ok := func() ([]packet.PlayerListItemEntry, bool) {
		t.mu.Lock()
		defer t.mu.Unlock()
		if len(t.entries) == 0 {
			// already empty
			return nil, false
		}
		items := make([]packet.PlayerListItemEntry, 0, len(t.entries))
		for _, e := range t.entries {
			items = append(items, *newPlayerListItemEntry(e))
		}
		t.entries = map[uuid.UUID]*tabListEntry{} // clear
		return items, true
	}()
	if !ok {
		return nil
	}

	return t.c.WritePacket(&packet.PlayerListItem{
		Action: packet.RemovePlayerListItemAction,
		Items:  items,
	})
}

func (t *tabList) HasEntry(id uuid.UUID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.hasEntry(id)
}
func (t *tabList) hasEntry(id uuid.UUID) bool {
	_, ok := t.entries[id]
	return ok
}

// Processes a tab list entry packet sent from the backend to the client.
func (t *tabList) processBackendPacket(p *packet.PlayerListItem) error {
	// Packet is already forwarded on, so no need to do that here
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, item := range p.Items {
		if item.ID == uuid.Nil {
			return errors.New("1.7 tab list entry given to modern tab list handler")
		}

		if p.Action != packet.AddPlayerListItemAction && !t.hasEntry(item.ID) {
			// Sometimes UpdateGameMode is sent before AddPlayer so don't want to warn here
			continue
		}

		switch p.Action {
		case packet.AddPlayerListItemAction:
			// ensure that name and properties are available
			if item.Name == "" || item.Properties == nil {
				return errors.New("got null game profile for AddPlayerListItemAction")
			}
			t.entries[item.ID] = &tabListEntry{
				tabList: t,
				profile: &profile.GameProfile{
					ID:         item.ID,
					Name:       item.Name,
					Properties: item.Properties,
				},
				displayName: item.DisplayName,
				latency:     time.Millisecond * time.Duration(item.Latency),
				gameMode:    item.GameMode,
			}
		case packet.RemovePlayerListItemAction:
			delete(t.entries, item.ID)
		case packet.UpdateDisplayNamePlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				e.setDisplayName(item.DisplayName)
			}
		case packet.UpdateLatencyPlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				e.setLatency(time.Millisecond * time.Duration(item.Latency))
			}
		case packet.UpdateGameModePlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				return e.setGameMode(item.GameMode)
			}
		default:
			// Nothing we can do here
		}
	}
	return nil
}

func (t *tabList) updateEntry(action packet.PlayerListItemAction, entry *tabListEntry) error {
	if _, ok := t.entries[entry.Profile().ID]; !ok {
		return nil
	}
	packetItem := newPlayerListItemEntry(entry)
	return t.c.WritePacket(&packet.PlayerListItem{
		Action: action,
		Items:  []packet.PlayerListItemEntry{*packetItem},
	})
}

//
//
//
//

type tabListEntry struct {
	tabList *tabList

	mu          sync.RWMutex // protects following fields
	profile     *profile.GameProfile
	displayName component.Component
	latency     time.Duration
	gameMode    int
}

var _ player.TabListEntry = (*tabListEntry)(nil)

func (t *tabListEntry) TabList() player.TabList {
	return t.tabList
}

func (t *tabListEntry) Profile() profile.GameProfile {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return *t.profile
}

func (t *tabListEntry) DisplayName() component.Component {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.displayName
}

func (t *tabListEntry) SetDisplayName(name component.Component) error {
	t.setDisplayName(name)
	return t.tabList.updateEntry(packet.UpdateDisplayNamePlayerListItemAction, t)
}

func (t *tabListEntry) setDisplayName(name component.Component) {
	t.mu.Lock()
	t.displayName = name
	t.mu.Unlock()
}

func (t *tabListEntry) GameMode() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.gameMode
}

func (t *tabListEntry) Latency() time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.latency
}

func (t *tabListEntry) setLatency(latency time.Duration) {
	t.mu.Lock()
	t.latency = latency
	t.mu.Unlock()
}

func (t *tabListEntry) setGameMode(gameMode int) error {
	t.mu.Lock()
	t.gameMode = gameMode
	t.mu.Unlock()
	return t.tabList.updateEntry(packet.UpdateGameModePlayerListItemAction, t)
}

func newPlayerListItemEntry(entry player.TabListEntry) *packet.PlayerListItemEntry {
	p := entry.Profile()
	return &packet.PlayerListItemEntry{
		ID:          p.ID,
		Name:        p.Name,
		Properties:  p.Properties,
		GameMode:    entry.GameMode(),
		Latency:     int(entry.Latency().Milliseconds()),
		DisplayName: entry.DisplayName(),
	}
}

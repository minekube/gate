package proxy

import (
	"bytes"
	"fmt"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/proto/packet"
	"go.minekube.com/gate/pkg/proxy/player"
	"go.minekube.com/gate/pkg/util"
	"go.minekube.com/gate/pkg/util/profile"
	"go.minekube.com/gate/pkg/util/uuid"
	"sync"
	"time"
)

type tabList struct {
	*minecraftConn

	mu      sync.RWMutex
	entries map[uuid.UUID]*tabListEntry
}

var _ player.TabList = (*tabList)(nil)

func newTabList(c *minecraftConn) *tabList {
	return &tabList{minecraftConn: c, entries: map[uuid.UUID]*tabListEntry{}}
}

func (t *tabList) SetHeaderFooter(header, footer component.Component) error {
	b := new(bytes.Buffer)
	p := new(packet.HeaderAndFooter)
	j := util.JsonCodec(t.protocol)

	if err := j.Marshal(b, header); err != nil {
		return fmt.Errorf("error marshal header: %w", err)
	}
	p.Header = b.String()
	b.Reset()
	if err := j.Marshal(b, footer); err != nil {
		return fmt.Errorf("error marshal footer: %w", err)
	}
	p.Footer = b.String()

	return t.WritePacket(p)
}

func (t *tabList) ClearHeaderFooter() error {
	return t.WritePacket(packet.ResetHeaderAndFooter)
}

func (t *tabList) clearAll() error {
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

	return t.WritePacket(&packet.PlayerListItem{
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
func (t *tabList) processBackendPacket(p *packet.PlayerListItem) {
	// Packet is already forwarded on, so no need to do that here
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, item := range p.Items {
		if p.Action != packet.AddPlayerListItemAction && !t.hasEntry(item.ID) {
			// Sometimes UpdateGameMode is sent before AddPlayer so don't want to warn here
			continue
		}

		switch p.Action {
		case packet.AddPlayerListItemAction:
			t.entries[item.ID] = &tabListEntry{
				profile: &profile.GameProfile{
					Id:         item.ID,
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
				e.SetDisplayName(item.DisplayName)
			}
		case packet.UpdateLatencyPlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				e.SetLatency(time.Millisecond * time.Duration(item.Latency))
			}
		case packet.UpdateGameModePlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				e.setGameMode(item.GameMode)
			}
		default:
			// Nothing we can do here
		}
	}
}

//
//
//
//

type tabListEntry struct {
	mu          sync.RWMutex // protects following fields
	profile     *profile.GameProfile
	displayName component.Component
	latency     time.Duration
	gameMode    int
}

var _ player.TabListEntry = (*tabListEntry)(nil)

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
func (t *tabListEntry) SetDisplayName(name component.Component) {
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

func (t *tabListEntry) SetLatency(latency time.Duration) {
	t.mu.Lock()
	t.latency = latency
	t.mu.Unlock()
}

func (t *tabListEntry) setGameMode(gameMode int) {
	t.mu.Lock()
	t.gameMode = gameMode
	t.mu.Unlock()
}

func newPlayerListItemEntry(entry player.TabListEntry) *packet.PlayerListItemEntry {
	p := entry.Profile()
	return &packet.PlayerListItemEntry{
		ID:          p.Id,
		Name:        p.Name,
		Properties:  p.Properties,
		GameMode:    entry.GameMode(),
		Latency:     int(entry.Latency() * time.Millisecond),
		DisplayName: entry.DisplayName(),
	}
}

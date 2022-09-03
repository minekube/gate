package tablist

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/util"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto/keyrevision"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
	"go.uber.org/multierr"
)

// TabList is the tab list of a player.
type TabList interface {
	SetHeaderFooter(header, footer component.Component) error // Sets the tab list header and footer for the player.
	ClearHeaderFooter() error                                 // Clears the tab list header and footer for the player.
	AddEntry(Entry) error                                     // Adds an entry to the tab list.
	RemoveEntry(id uuid.UUID) error                           // Removes an entry from the tab list.
	HasEntry(id uuid.UUID) bool                               // Determines if the specified entry exists in the tab list.
	Entries() map[uuid.UUID]Entry                             // Returns the entries in the tab list.
	ProcessBackendPacket(*packet.PlayerListItem) error        // Processes a packet.PlayerListItem sent from the backend to the client.
}

type (
	// tabList is the tab list of one player connection.
	tabList struct {
		keyProvider PlayerKeyProvider
		w           proto.PacketWriter
		protocol    proto.Protocol

		mu      sync.RWMutex
		entries map[uuid.UUID]*tabListEntry
	}
	PlayerKeyProvider interface {
		PlayerKey(playerID uuid.UUID) crypto.IdentifiedKey // May return nil if player not found
	}
)

var _ TabList = (*tabList)(nil)

// New creates a new TabList for versions >= 1.8.
func New(w proto.PacketWriter, protocol proto.Protocol, keyProvider PlayerKeyProvider) TabList {
	return newTabList(w, protocol, keyProvider)
}

func newTabList(w proto.PacketWriter, protocol proto.Protocol, keyProvider PlayerKeyProvider) *tabList {
	return &tabList{
		keyProvider: keyProvider,
		w:           w,
		protocol:    protocol,
		entries:     make(map[uuid.UUID]*tabListEntry),
	}
}

func (t *tabList) Entries() map[uuid.UUID]Entry {
	t.mu.RLock()
	entries := t.entries
	t.mu.RUnlock()
	// Convert to TabListEntry interface
	m := make(map[uuid.UUID]Entry, len(entries))
	for id, e := range entries {
		m[id] = e
	}
	return m
}

func (t *tabList) AddEntry(entry Entry) error {
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
	if t.hasEntry(entry.Profile().ID) {
		t.mu.Unlock()
		return errors.New("tab list already has entry of same profile id")
	}
	t.entries[entry.Profile().ID] = e
	t.mu.Unlock()
	return t.w.WritePacket(&packet.PlayerListItem{
		Action: packet.AddPlayerListItemAction,
		Items:  []packet.PlayerListItemEntry{*newPlayerListItemEntry(entry)},
	})
}

func (t *tabList) RemoveEntry(id uuid.UUID) error {
	_, err := t.removeEntry(id)
	return err
}

func (t *tabList) removeEntry(id uuid.UUID) (*tabListEntry, error) {
	t.mu.Lock()
	entry, ok := t.entries[id]
	delete(t.entries, id)
	t.mu.Unlock()
	if ok {
		return entry, t.w.WritePacket(&packet.PlayerListItem{
			Action: packet.RemovePlayerListItemAction,
			Items:  []packet.PlayerListItemEntry{*newPlayerListItemEntry(entry)},
		})
	}
	return entry, nil
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

	return t.w.WritePacket(p)
}

func (t *tabList) ClearHeaderFooter() error {
	return t.w.WritePacket(packet.ResetHeaderAndFooter)
}

// removes all player entries shown in the tab list
func (t *tabList) clearEntries(bufferPacket func(proto.Packet) error) error {
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

	return bufferPacket(&packet.PlayerListItem{
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

func (t *tabList) ProcessBackendPacket(p *packet.PlayerListItem) error {
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

			if _, ok := t.entries[item.ID]; !ok {
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
					playerKey:   providedKey,
				}
			}
		case packet.RemovePlayerListItemAction:
			delete(t.entries, item.ID)
		case packet.UpdateDisplayNamePlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				e.setDisplayNameNoUpdate(item.DisplayName)
			}
		case packet.UpdateLatencyPlayerListItemAction:
			e, ok := t.entries[item.ID]
			if ok {
				e.setLatency(time.Millisecond * time.Duration(item.Latency))
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
	return nil
}

func (t *tabList) updateEntry(action packet.PlayerListItemAction, entry *tabListEntry) error {
	if !t.HasEntry(entry.Profile().ID) {
		return nil
	}
	packetItem := newPlayerListItemEntry(entry)

	selectedKey := packetItem.PlayerKey
	if existing := t.keyProvider.PlayerKey(entry.Profile().ID); existing != nil {
		selectedKey = existing
	}

	if selectedKey != nil &&
		selectedKey.SignatureHolder() == entry.Profile().ID &&
		keyrevision.Applicable(selectedKey.KeyRevision(), t.protocol) {
		packetItem.PlayerKey = selectedKey
	} else {
		packetItem.PlayerKey = nil
	}

	return t.w.WritePacket(&packet.PlayerListItem{
		Action: action,
		Items:  []packet.PlayerListItemEntry{*packetItem},
	})
}

func (t *tabList) entry(id uuid.UUID) *tabListEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.entries[id]
}

// BufferClearTabListEntries clears all entries from the tab list.
// The packet entries are written with bufferPacket, so make sure to do an explicit flush.
func BufferClearTabListEntries(list TabList, bufferPacket func(proto.Packet) error) error {
	if internal, ok := list.(tabListInternal); ok {
		return internal.clearEntries(bufferPacket)
	}
	// fallback implementation
	var err error
	for id := range list.Entries() {
		err = multierr.Append(err, list.RemoveEntry(id))
	}
	return err
}

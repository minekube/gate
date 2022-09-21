package tablist

import (
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Entry is a single entry/player in a TabList.
type Entry interface {
	TabList() TabList // The TabList this entry is in.
	// Profile returns the profile of the entry, which uniquely identifies the entry with its
	// containing uuid, as well as deciding what is shown as the player head in the tab list.
	Profile() profile.GameProfile
	// DisplayName returns the optional text displayed for this entry in the TabList,
	// otherwise if returns nil Profile().Name is shown (but not returned here).
	DisplayName() component.Component
	// SetDisplayName the text to be displayed for the entry.
	// If nil Profile().Name will be shown.
	SetDisplayName(component.Component) error
	// GameMode returns the game mode the entry has been set to.
	//  0 - Survival
	//  1 - Creative
	//  2 - Adventure
	//  3 - Spectator
	GameMode() int
	// SetGameMode sets the gamemode for the entry.
	// See GameMode() for more details.
	SetGameMode(int) error
	// Latency returns the latency/ping for the entry.
	//
	// The icon shown in the tab list is calculated
	// by the millisecond latency as follows:
	//
	//  A negative latency will display the no connection icon
	//  0-150 will display 5 bars
	//  150-300 will display 4 bars
	//  300-600 will display 3 bars
	//  600-1000 will display 2 bars
	//  A latency move than 1 second will display 1 bar
	Latency() time.Duration
	// SetLatency sets the latency/ping for the entry.
	// See Latency() for how it is displayed.
	SetLatency(time.Duration) error
	crypto.KeyIdentifiable
}

type tabListInternal interface {
	TabList
	updateEntry(action packet.PlayerListItemAction, entry *tabListEntry) error
	clearEntries(bufferPacket func(proto.Packet) error) error
}

type tabListEntry struct {
	tabList tabListInternal

	onSetDisplayName func() // hook

	mu          sync.RWMutex // protects following fields
	profile     *profile.GameProfile
	displayName component.Component // nil-able
	latency     time.Duration
	gameMode    int
	// This is only intended and only works for players currently not connected to this proxy.
	// For any player currently connected to this proxy this will be filled automatically.
	// Will ignore mismatching key revision data.
	playerKey crypto.IdentifiedKey // nil-able
}

var _ Entry = (*tabListEntry)(nil)

func (t *tabListEntry) TabList() TabList {
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
	if fn := t.onSetDisplayName; fn != nil {
		fn()
	}
	t.setDisplayNameNoUpdate(name)
	return t.tabList.updateEntry(packet.UpdateDisplayNamePlayerListItemAction, t)
}

func (t *tabListEntry) setDisplayNameNoUpdate(name component.Component) {
	t.mu.Lock()
	if name == nil {
		name = &component.Text{Content: t.profile.Name}
	}
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

func (t *tabListEntry) SetLatency(latency time.Duration) error {
	t.setLatency(latency)
	return t.tabList.updateEntry(packet.UpdateLatencyPlayerListItemAction, t)
}
func (t *tabListEntry) setLatency(latency time.Duration) {
	t.mu.Lock()
	t.latency = latency
	t.mu.Unlock()
}

func (t *tabListEntry) SetGameMode(gameMode int) error {
	t.setGameMode(gameMode)
	return t.tabList.updateEntry(packet.UpdateGameModePlayerListItemAction, t)
}
func (t *tabListEntry) setGameMode(gameMode int) {
	t.mu.Lock()
	t.gameMode = gameMode
	t.mu.Unlock()
}

func (t *tabListEntry) IdentifiedKey() crypto.IdentifiedKey {
	return t.playerKey
}

func newPlayerListItemEntry(entry Entry) *packet.PlayerListItemEntry {
	p := entry.Profile()
	return &packet.PlayerListItemEntry{
		ID:          p.ID,
		Name:        p.Name,
		Properties:  p.Properties,
		GameMode:    entry.GameMode(),
		Latency:     int(entry.Latency().Milliseconds()),
		DisplayName: entry.DisplayName(),
		PlayerKey:   entry.IdentifiedKey(),
	}
}

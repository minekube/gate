package tablist

import (
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
)

type KeyedEntry struct {
	Entry Entry
}

var _ tablist.Entry = (*KeyedEntry)(nil)

func (k *KeyedEntry) TabList() tablist.TabList         { return k.Entry.TabList() }
func (k *KeyedEntry) Profile() profile.GameProfile     { return k.Entry.Profile() }
func (k *KeyedEntry) DisplayName() component.Component { return k.Entry.DisplayName() }
func (k *KeyedEntry) GameMode() int                    { return k.Entry.GameMode() }
func (k *KeyedEntry) Latency() time.Duration           { return k.Entry.Latency() }
func (k *KeyedEntry) ChatSession() player.ChatSession  { return k.Entry.ChatSession() }
func (k *KeyedEntry) Listed() bool                     { return k.Entry.Listed() }

func (k *KeyedEntry) SetDisplayName(name component.Component) error {
	k.Entry.Lock()
	k.Entry.EntryAttributes.DisplayName = name
	k.Entry.Unlock()
	return k.Entry.OwningTabList.UpdateEntry(legacytablist.UpdateDisplayNamePlayerListItemAction, k)
}

func (k *KeyedEntry) SetGameMode(gameMode int) error {
	k.Entry.Lock()
	k.Entry.EntryAttributes.GameMode = gameMode
	k.Entry.Unlock()
	return k.Entry.OwningTabList.UpdateEntry(legacytablist.UpdateGameModePlayerListItemAction, k)
}

func (k *KeyedEntry) SetLatency(latency time.Duration) error {
	k.Entry.Lock()
	k.Entry.EntryAttributes.Latency = latency
	k.Entry.Unlock()
	return k.Entry.OwningTabList.UpdateEntry(legacytablist.UpdateLatencyPlayerListItemAction, k)
}

func (k *KeyedEntry) SetListed(bool) error {
	return nil // not supported
}

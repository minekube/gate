package tablist

import (
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
)

type KeyedEntry struct {
	Entry
}

var _ tablist.Entry = (*KeyedEntry)(nil)

func (k *KeyedEntry) SetDisplayName(name component.Component) error {
	k.SetDisplayNameInternal(name)
	return k.Entry.OwningTabList.UpdateEntry(legacytablist.UpdateDisplayNamePlayerListItemAction, k)
}
func (k *KeyedEntry) SetGameMode(gameMode int) error {
	k.SetGameModeInternal(gameMode)
	return k.Entry.OwningTabList.UpdateEntry(legacytablist.UpdateGameModePlayerListItemAction, k)
}
func (k *KeyedEntry) SetLatency(latency time.Duration) error {
	k.SetLatencyInternal(latency)
	return k.Entry.OwningTabList.UpdateEntry(legacytablist.UpdateLatencyPlayerListItemAction, k)
}

func (k *KeyedEntry) SetListed(bool) error {
	return nil // not supported
}

package tablist

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/edition/java/proxy/tablist"
	"go.minekube.com/gate/pkg/util/uuid"
)

type EntryAttributes struct {
	Profile     profile.GameProfile
	DisplayName component.Component // nil-able
	Latency     time.Duration
	GameMode    int
	Listed      bool
	ChatSession player.ChatSession
}

type Entry struct {
	OwningTabList InternalTabList

	sync.RWMutex
	EntryAttributes
}

var _ tablist.Entry = (*Entry)(nil)
var _ internalEntry = (*Entry)(nil)

type internalEntry interface {
	SetDisplayNameInternal(name component.Component)
	SetLatencyInternal(latency time.Duration)
	SetGameModeInternal(gameMode int)
	SetChatSessionInternal(chatSession player.ChatSession)
	SetListedInternal(listed bool)
}

func doInternalEntity(e tablist.Entry, fn func(internalEntry)) {
	if i, ok := e.(internalEntry); ok {
		fn(i)
	}
}

func (e *Entry) TabList() tablist.TabList {
	return e.OwningTabList
}

func (e *Entry) Profile() profile.GameProfile {
	e.RLock()
	defer e.RUnlock()
	return e.EntryAttributes.Profile
}

func (e *Entry) DisplayName() component.Component {
	e.RLock()
	defer e.RUnlock()
	return e.EntryAttributes.DisplayName
}

func (e *Entry) SetDisplayNameInternal(name component.Component) {
	e.Lock()
	e.EntryAttributes.DisplayName = name
	e.Unlock()
}
func (e *Entry) SetDisplayName(name component.Component) error {
	e.Lock()
	e.EntryAttributes.DisplayName = name
	profileID := e.EntryAttributes.Profile.ID
	e.Unlock()
	upsertEntry, err := rawEntry(profileID)
	if err != nil {
		return fmt.Errorf("error creating upsert entry: %w", err)
	}
	upsertEntry.DisplayName = name
	return e.OwningTabList.EmitActionRaw(playerinfo.UpdateDisplayNameAction, upsertEntry)
}

func (e *Entry) GameMode() int {
	e.RLock()
	defer e.RUnlock()
	return e.EntryAttributes.GameMode
}

func (e *Entry) SetGameModeInternal(gameMode int) {
	e.Lock()
	e.EntryAttributes.GameMode = gameMode
	e.Unlock()
}
func (e *Entry) SetGameMode(gameMode int) error {
	e.Lock()
	e.EntryAttributes.GameMode = gameMode
	profileID := e.EntryAttributes.Profile.ID
	e.Unlock()
	upsertEntry, err := rawEntry(profileID)
	if err != nil {
		return fmt.Errorf("error creating upsert entry: %w", err)
	}
	upsertEntry.GameMode = gameMode
	return e.OwningTabList.EmitActionRaw(playerinfo.UpdateGameModeAction, upsertEntry)
}

func (e *Entry) Latency() time.Duration {
	e.TryRLock()
	e.RLock()
	defer e.RUnlock()
	return e.EntryAttributes.Latency
}

func (e *Entry) SetLatencyInternal(latency time.Duration) {
	e.Lock()
	e.EntryAttributes.Latency = latency
	e.Unlock()
}
func (e *Entry) SetLatency(latency time.Duration) error {
	e.Lock()
	e.EntryAttributes.Latency = latency
	profileID := e.EntryAttributes.Profile.ID
	e.Unlock()
	upsertEntry, err := rawEntry(profileID)
	if err != nil {
		return fmt.Errorf("error creating upsert entry: %w", err)
	}
	upsertEntry.Latency = int(latency.Milliseconds())
	return e.OwningTabList.EmitActionRaw(playerinfo.UpdateLatencyAction, upsertEntry)
}

func (e *Entry) ChatSession() player.ChatSession {
	e.RLock()
	defer e.RUnlock()
	return e.EntryAttributes.ChatSession
}

func (e *Entry) SetChatSessionInternal(chatSession player.ChatSession) {
	e.Lock()
	e.EntryAttributes.ChatSession = chatSession
	e.Unlock()
}

func (e *Entry) Listed() bool {
	e.RLock()
	defer e.RUnlock()
	return e.EntryAttributes.Listed
}

func (e *Entry) SetListedInternal(listed bool) {
	e.Lock()
	e.EntryAttributes.Listed = listed
	e.Unlock()
}
func (e *Entry) SetListed(listed bool) error {
	e.Lock()
	e.EntryAttributes.Listed = listed
	profileID := e.EntryAttributes.Profile.ID
	e.Unlock()
	upsertEntry, err := rawEntry(profileID)
	if err != nil {
		return fmt.Errorf("error creating upsert entry: %w", err)
	}
	upsertEntry.Listed = listed
	return e.OwningTabList.EmitActionRaw(playerinfo.UpdateListedAction, upsertEntry)
}

func rawEntry(profileID uuid.UUID) (*playerinfo.Entry, error) {
	if profileID == uuid.Nil {
		return nil, errors.New("profile id must not be zero")
	}
	return &playerinfo.Entry{
		ProfileID: profileID,
	}, nil
}

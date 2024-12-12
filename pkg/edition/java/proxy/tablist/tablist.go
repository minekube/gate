package tablist

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/edition/java/proxy/player"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// TabList is the tab list of a player.
type TabList interface {
	Add(entries ...Entry) error       // Adds one or more entries to the tab list.
	RemoveAll(ids ...uuid.UUID) error // Removes one or more entries from the tab list. If empty removes all entries.
	Entries() map[uuid.UUID]Entry     // Returns the entries in the tab list.
	// SetHeaderFooter sets the header and footer of the tab list.
	//
	// If nil is passed for either, the header/footer will be cleared.
	// Use ClearTabListHeaderFooter() to clear the header and footer for convenience.
	SetHeaderFooter(header, footer component.Component) error
	HeaderFooter() (header, footer component.Component) // Returns the header and footer of the tab list. May be nil if not set.
}

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
	// ChatSession returns the chat session associated with this entry.
	ChatSession() player.ChatSession
	// Listed indicates whether the entry is listed,
	// when listed they will be visible to other players in the tab list.
	Listed() bool
	// SetListed sets whether the entry is listed.
	// Only changeable in 1.19.3 and above!
	SetListed(bool) error
}

// Viewer is a tab list viewer (player).
type Viewer interface {
	proto.PacketWriter
	Protocol() proto.Protocol
	IdentifiedKey() crypto.IdentifiedKey
}

// SendHeaderFooter updates the tab list header and footer for a Viewer.
func SendHeaderFooter(viewer Viewer, header, footer component.Component) error {
	return viewer.WritePacket(&packet.HeaderAndFooter{
		Header: *chat.FromComponentProtocol(header, viewer.Protocol()),
		Footer: *chat.FromComponentProtocol(footer, viewer.Protocol()),
	})
}

// ClearTabListHeaderFooter clears the tab list header and footer for a tab list.
// Convenience function for tabList.SetHeaderFooter(nil, nil).
func ClearTabListHeaderFooter(tabList TabList) error {
	return tabList.SetHeaderFooter(nil, nil)
}

// ClearHeaderFooter clears the tab list header and footer for the viewer.
func ClearHeaderFooter(viewer proto.PacketWriter) error {
	return viewer.WritePacket(packet.ResetHeaderAndFooter)
}

// HasEntry determines if the specified entry exists in the tab list.
func HasEntry(tl TabList, id uuid.UUID) bool {
	_, ok := tl.Entries()[id]
	return ok
}

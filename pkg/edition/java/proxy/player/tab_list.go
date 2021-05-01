package player

import (
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/util/uuid"
	"time"
)

// TabList is the tab list of a proxy.Player.
type TabList interface {
	SetHeaderFooter(header, footer component.Component) error // Sets the tab list header and footer for the player.
	ClearHeaderFooter() error                                 // Clears the tab list header and footer for the player.
	AddEntry(TabListEntry) error                              // Adds an entry to the tab list.
	RemoveEntry(id uuid.UUID) error                           // Removes an entry from the tab list.
	// Determines if the specified entry exists in the tab list.
	HasEntry(id uuid.UUID) bool
	// TODO Entries() []TabListEntry
}

// TabListEntry is a single entry in a TabList.
type TabListEntry interface {
	TabList() TabList // The TabList this entry is in.
	// Returns the profile of the entry, which uniquely identifies the entry with its
	// containing uuid, as well as deciding what is shown as the player head in the tab list.
	Profile() profile.GameProfile
	// Returns the optional text displayed for this entry in the TabList,
	// otherwise profile.GameProfile.Names is shown and returns nil.
	DisplayName() component.Component
	// Gets the game mode the entry has been set to.
	GameMode() int
	// Returns the latency for the entry.
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
	//
	Latency() time.Duration
	// TODO SetDisplayName
	// TODO SetLatency(time.Duration)
	// TODO more...
}

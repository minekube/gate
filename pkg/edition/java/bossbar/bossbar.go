// Package bossbar provides a way to create and manage Minecraft boss bars for players.
package bossbar

import (
	"context"

	"go.minekube.com/common/minecraft/component"
	packet "go.minekube.com/gate/pkg/edition/java/proto/packet/bossbar"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// BossBar is a boss bar.
type BossBar interface {
	// Viewers returns the viewers of this boss bar.
	Viewers() []Viewer
	// AddViewer adds the specified viewer to the boss bar's viewers
	AddViewer(Viewer) error
	// RemoveViewer removes the specified viewer from the boss bar's viewers
	RemoveViewer(Viewer) error

	// ID returns the boss bar's ID.
	ID() uuid.UUID
	// Name returns the name of this boss bar.
	Name() component.Component
	// SetName sets the name of this boss bar.
	SetName(component.Component)

	// Color returns the color of this boss bar.
	Color() Color
	// SetColor sets the color of this boss bar.
	SetColor(Color)

	// Percent returns the percent of this boss bar.
	Percent() float32
	// SetPercent sets the percent of this boss bar.
	SetPercent(float32)

	// Flags returns the flags of this boss bar.
	Flags() []Flag
	// SetFlags sets the flags of this boss bar.
	SetFlags([]Flag)

	// Overlay returns the overlay of this boss bar.
	Overlay() Overlay
	// SetOverlay sets the overlay of this boss bar.
	SetOverlay(Overlay)
}

// New creates a new boss bar.
// It is safe for concurrent use.
func New(
	name component.Component,
	percent float32,
	color Color,
	overlay Overlay,
	flags ...Flag,
) BossBar {
	return &bossBar{
		viewers: make(map[uuid.UUID]*barViewer),
		BossBar: packet.BossBar{
			ID:      uuid.New(),
			Name:    chat.FromComponent(name),
			Percent: percent,
			Color:   color,
			Overlay: overlay,
			Flags:   packet.ConvertFlags(flags...),
		},
		flags: flags,
	}
}

// RemoveAllViewers removes all viewers from the boss bar.
func RemoveAllViewers(b BossBar) {
	for _, viewer := range b.Viewers() {
		go func(v Viewer) { _ = b.RemoveViewer(v) }(viewer)
	}
}

// Viewer is the interface for a boss bar viewer (e.g. a player).
type Viewer interface {
	ID() uuid.UUID
	Context() context.Context
	proto.PacketWriter
}

// ManagedViewer is an optional interface for viewers that support boss bar management
// during server transitions (1.20.2+). If a viewer implements this interface, the boss bar
// will register/unregister with the viewer's manager and use WriteBossBarPacket for packets.
type ManagedViewer interface {
	Viewer
	// RegisterBossBar registers a boss bar with this viewer's manager.
	RegisterBossBar(bar BossBar)
	// UnregisterBossBar unregisters a boss bar from this viewer's manager.
	UnregisterBossBar(bar BossBar)
	// WriteBossBarPacket writes a boss bar packet, respecting the dropping state.
	// Returns true if the packet was written (or dropped intentionally).
	WriteBossBarPacket(p *packet.BossBar) bool
}

// Color is the color of the percent bar.
type Color = packet.Color

// Available boss bar colors.
const (
	PinkColor   = Color(packet.PinkColor)
	BlueColor   = Color(packet.BlueColor)
	RedColor    = Color(packet.RedColor)
	GreenColor  = Color(packet.GreenColor)
	YellowColor = Color(packet.YellowColor)
	PurpleColor = Color(packet.PurpleColor)
	WhiteColor  = Color(packet.WhiteColor)
)

// Colors is a list of available boss bar colors.
var Colors = []Color{
	PinkColor,
	BlueColor,
	RedColor,
	GreenColor,
	YellowColor,
	PurpleColor,
	WhiteColor,
}

const (
	// MinProgress is the minimum value the progress can be.
	MinProgress float32 = 0.0
	// MaxProgress is the maximum value the progress can be.
	MaxProgress float32 = 1.0
)

// Overlay is a boss bar overlay.
type Overlay = packet.Overlay

// Available boss bar overlays.
const (
	ProgressOverlay  = Overlay(packet.ProgressOverlay)
	Notched6Overlay  = Overlay(packet.Notched6Overlay)
	Notched10Overlay = Overlay(packet.Notched10Overlay)
	Notched12Overlay = Overlay(packet.Notched12Overlay)
	Notched20Overlay = Overlay(packet.Notched20Overlay)
)

// Flag is a boss bar flag.
type Flag = packet.Flag

// Available boss bar flags.
const (
	DarkenScreenFlag   = Flag(packet.DarkenScreenFlag)
	PlayBossMusicFlag  = Flag(packet.PlayBossMusicFlag)
	CreateWorldFogFlag = Flag(packet.CreateWorldFogFlag)
)

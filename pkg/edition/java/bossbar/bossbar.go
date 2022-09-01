package bossbar

import (
	"go.minekube.com/common/minecraft/color"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
)

type Manager interface {
	// Add adds the specified viewer to the boss bar's viewers
	// and spawns the boss bar, registering the boss bar if needed.
	Add(viewer Viewer, bar BossBar) error
	// Remove is called when a viewer disconnects from the proxy.
	// Removes the player from any boss bar subscriptions.
	Remove(viewer Viewer, bar BossBar) error
	Broadcast(bar BossBar) error
}

type BossBar interface {
	Name() component.Component
	SetName(component.Component)
}

// Viewer is the interface for a boss bar viewer.
type Viewer interface {
	proto.PacketWriter
}

const (
	// MinProgress is the minimum value the progress can be.
	MinProgress float32 = 0.0
	// MaxProgress is the maximum value the progress can be.
	MaxProgress float32 = 1.0
)

// Color is a color for a boss bar.
type Color interface {
	color.Color
	ID() ColorID
}

// ColorID is the id of a boss bar color.
type ColorID = packet.BossBarColor

// Available boss bar colors.
var (
	Pink   Color = &barColor{Color: color.LightPurple, id: packet.BossBarColorPink}
	Blue   Color = &barColor{Color: color.Blue, id: packet.BossBarColorBlue}
	Red    Color = &barColor{Color: color.Red, id: packet.BossBarColorRed}
	Green  Color = &barColor{Color: color.Green, id: packet.BossBarColorGreen}
	Yellow Color = &barColor{Color: color.Yellow, id: packet.BossBarColorYellow}
	Purple Color = &barColor{Color: color.DarkPurple, id: packet.BossBarColorPurple}
	White  Color = &barColor{Color: color.White, id: packet.BossBarColorWhite}
)

// Colors is a list of available boss bar colors.
var Colors = []Color{Pink, Blue, Red, Green, Yellow, Purple, White}

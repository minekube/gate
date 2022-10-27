// Package title provides functionality for showing Minecraft titles for players.
package title

import (
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/internal/protoutil"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
)

// Viewer is the interface for a title viewer (e.g. a player).
type Viewer interface {
	netmc.PacketWriter
}

// ClearTitle clears the title of the viewer.
func ClearTitle(viewer Viewer) error {
	protocol, ok := isProtocolSupported(viewer)
	if !ok {
		return nil
	}
	reset, err := title.New(protocol, &title.Builder{Action: title.Reset})
	if err != nil {
		return err
	}
	return viewer.WritePacket(reset)
}

// HideTitle hides the title from the viewer.
func HideTitle(viewer Viewer) error {
	protocol, ok := isProtocolSupported(viewer)
	if !ok {
		return nil
	}
	reset, err := title.New(protocol, &title.Builder{Action: title.Hide})
	if err != nil {
		return err
	}
	return viewer.WritePacket(reset)
}

// Options for showing a title.
type Options struct {
	Title                 component.Component
	Subtitle              component.Component
	FadeIn, Stay, FadeOut time.Duration
	// The parts of a title to update exclusively.
	// If empty, all parts are being updated.
	Parts []Part
}

// Part is a part of a title.
type Part byte

const (
	TitlePart    Part = iota + 1 // Update title
	SubtitlePart                 // Update subtitle
	TimesPart                    // Update title times
)

// ShowTitle shows a title to the viewer.
func ShowTitle(viewer Viewer, opts *Options) error {
	if opts == nil {
		return nil
	}
	protocol, ok := isProtocolSupported(viewer)
	if !ok {
		return nil
	}

	hasPart := func(p Part) bool {
		if len(opts.Parts) == 0 {
			return true // empty means all parts
		}
		for _, part := range opts.Parts {
			if part == p {
				return true
			}
		}
		return false
	}

	if hasPart(TimesPart) {
		fadeIn := opts.FadeIn
		stay := opts.Stay
		fadeOut := opts.FadeOut
		if fadeIn == 0 && stay == 0 && fadeOut == 0 {
			// Set defaults
			fadeIn = time.Second / 2
			stay = time.Second * 3
			fadeOut = time.Second / 2
		}
		timesPkt, err := title.New(protocol, &title.Builder{
			Action: title.SetTimes,
			// 50 = 1000ms / 20 ticks per second
			FadeIn:  int(fadeIn.Milliseconds() / 50),
			Stay:    int(stay.Milliseconds() / 50),
			FadeOut: int(fadeOut.Milliseconds() / 50),
		})
		if err != nil {
			return err
		}
		if err = viewer.BufferPacket(timesPkt); err != nil {
			return err
		}
	}

	if hasPart(SubtitlePart) {
		subtitle := opts.Subtitle
		if subtitle == nil {
			subtitle = empty
		}
		subtitlePkt, err := title.New(protocol, &title.Builder{
			Action:    title.SetSubtitle,
			Component: subtitle,
		})
		if err != nil {
			return err
		}
		if err = viewer.BufferPacket(subtitlePkt); err != nil {
			return err
		}
	}

	if hasPart(TitlePart) {
		ti := opts.Title
		if ti == nil {
			ti = empty
		}
		titlePkt, err := title.New(protocol, &title.Builder{
			Action:    title.SetTitle,
			Component: ti,
		})
		if err != nil {
			return err
		}
		if err = viewer.BufferPacket(titlePkt); err != nil {
			return err
		}
	}

	return viewer.Flush()
}

var empty = &component.Text{}

func isProtocolSupported(viewer Viewer) (proto.Protocol, bool) {
	protocol, _ := protoutil.Protocol(viewer)
	return protocol, protocol.GreaterEqual(version.Minecraft_1_8)
}

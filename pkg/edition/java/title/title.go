// Package title provides functionality for showing Minecraft titles for players.
package title

import (
	"time"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/internal/protoutil"
	"go.minekube.com/gate/pkg/edition/java/netmc"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/title"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

// Viewer is the interface for a title viewer (e.g. a player).
type Viewer interface {
	netmc.PacketWriter
}

// ResetTitle resets the title of the viewer.
func ResetTitle(viewer Viewer) error {
	return ShowTitle(viewer, nil)
}

// Options for showing a title.
type Options struct {
	Title                 component.Component
	Subtitle              component.Component
	FadeIn, Stay, FadeOut time.Duration
}

// ShowTitle shows a title to the viewer.
func ShowTitle(viewer Viewer, opts *Options) error {
	protocol, _ := protoutil.Protocol(viewer)
	if !protocol.GreaterEqual(version.Minecraft_1_8) {
		return nil
	}

	if opts == nil {
		reset, err := title.New(protocol, &title.Builder{Action: title.Reset})
		if err != nil {
			return err
		}
		return viewer.WritePacket(reset)
	}

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

	if err = viewer.BufferPacket(timesPkt); err != nil {
		return err
	}
	if err = viewer.BufferPacket(subtitlePkt); err != nil {
		return err
	}
	return viewer.WritePacket(titlePkt)
}

var empty = &component.Text{}

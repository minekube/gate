package bossbar

import (
	"context"
	"sync"

	"go.minekube.com/gate/pkg/edition/java/proto/packet/chat"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/internal/methods"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/bossbar"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type bossBar struct {
	mu      sync.RWMutex // protects following fields
	viewers map[uuid.UUID]*barViewer
	bossbar.BossBar
	flags []Flag
}

type barViewer struct {
	Viewer
	// canceled when removed from boss bar
	ctx     context.Context
	removed context.CancelFunc
}

func (b *bossBar) ID() uuid.UUID {
	return b.BossBar.ID // immutable, no lock needed
}

func (b *bossBar) RemoveViewer(viewer Viewer) error {
	b.mu.Lock()
	v, ok := b.viewers[viewer.ID()]
	if !ok {
		b.mu.Unlock()
		return nil
	}
	delete(b.viewers, v.ID())
	v.removed()
	p := b.createRemovePacket()
	b.mu.Unlock()

	return viewer.WritePacket(p)
}

func (b *bossBar) AddViewer(viewer Viewer) error {
	if !isProtocolSupported(viewer) {
		return nil
	}

	b.mu.Lock()
	_, ok := b.viewers[viewer.ID()]
	if ok {
		b.mu.Unlock()
		return nil
	}

	v := newBarViewer(viewer, b)

	b.viewers[viewer.ID()] = v
	p := b.createAddPacket()
	b.mu.Unlock()

	err := viewer.WritePacket(p)
	if err != nil {
		b.mu.Lock()
		delete(b.viewers, viewer.ID())
		b.mu.Unlock()
		v.removed()
		return err
	}

	return nil
}

func newBarViewer(viewer Viewer, bar *bossBar) *barViewer {
	removedCtx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		select {
		case <-viewer.Context().Done():
			_ = bar.RemoveViewer(viewer)
		case <-removedCtx.Done(): // viewer removed by RemoveViewer method
		}
	}()
	return &barViewer{
		Viewer:  viewer,
		ctx:     removedCtx,
		removed: cancel,
	}
}

func (b *bossBar) Viewers() []Viewer {
	b.mu.RLock()
	viewers := b.viewers
	b.mu.RUnlock()
	return toSlice(viewers)
}

var _ BossBar = (*bossBar)(nil)

func (b *bossBar) writeToViewers(p proto.Packet) {
	for _, viewer := range b.viewers {
		go func(v Viewer) { _ = v.WritePacket(p) }(viewer)
	}
}

func (b *bossBar) Name() component.Component {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.BossBar.Name.AsComponentOrNil()
}
func (b *bossBar) SetName(name component.Component) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if name == nil {
		return
	}
	b.BossBar.Name = chat.FromComponent(name)
	b.writeToViewers(b.createTitleUpdate(name))
}
func (b *bossBar) Color() Color {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.BossBar.Color
}
func (b *bossBar) SetColor(color Color) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.BossBar.Color == color {
		return
	}
	b.BossBar.Color = color
	b.writeToViewers(b.createColorUpdate(color))
}
func (b *bossBar) Percent() float32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.BossBar.Percent
}
func (b *bossBar) SetPercent(percent float32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if percent < MinProgress || percent > MaxProgress || b.BossBar.Percent == percent {
		return
	}
	b.BossBar.Percent = percent
	b.writeToViewers(b.createPercentUpdate(percent))
}
func (b *bossBar) Flags() []Flag {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.flags
}
func (b *bossBar) SetFlags(flags []Flag) {
	newFlags := bossbar.ConvertFlags(flags...)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.BossBar.Flags == newFlags {
		return
	}
	b.flags = flags
	b.BossBar.Flags = newFlags
	b.writeToViewers(b.createFlagsUpdate(newFlags))
}
func (b *bossBar) Overlay() Overlay {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.BossBar.Overlay
}
func (b *bossBar) SetOverlay(overlay Overlay) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.BossBar.Overlay == overlay {
		return
	}
	b.BossBar.Overlay = overlay
	b.writeToViewers(b.createOverlayUpdate(overlay))
}

func (b *bossBar) createRemovePacket() *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:     b.BossBar.ID,
		Action: bossbar.RemoveAction,
	}
}
func (b *bossBar) createAddPacket() *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:      b.BossBar.ID,
		Action:  bossbar.AddAction,
		Name:    b.BossBar.Name,
		Percent: b.BossBar.Percent,
		Color:   b.BossBar.Color,
		Overlay: b.BossBar.Overlay,
		Flags:   b.BossBar.Flags,
	}
}
func (b *bossBar) createPercentUpdate(percent float32) *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:      b.BossBar.ID,
		Action:  bossbar.UpdatePercentAction,
		Percent: percent,
	}
}
func (b *bossBar) createColorUpdate(color Color) *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:      b.BossBar.ID,
		Action:  bossbar.UpdateStyleAction,
		Color:   color,
		Overlay: b.BossBar.Overlay,
		Flags:   b.BossBar.Flags,
	}
}
func (b *bossBar) createOverlayUpdate(overlay Overlay) *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:      b.BossBar.ID,
		Action:  bossbar.UpdateStyleAction,
		Color:   b.BossBar.Color,
		Overlay: overlay,
	}
}
func (b *bossBar) createTitleUpdate(name component.Component) *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:     b.BossBar.ID,
		Action: bossbar.UpdateNameAction,
		Name:   chat.FromComponent(name),
	}
}
func (b *bossBar) createFlagsUpdate(flags byte) *bossbar.BossBar {
	return &bossbar.BossBar{
		ID:     b.BossBar.ID,
		Action: bossbar.UpdatePropertiesAction,
		Color:  b.BossBar.Color,
		Flags:  flags,
	}
}

func toSlice(m map[uuid.UUID]*barViewer) []Viewer {
	viewers := make([]Viewer, 0, len(m))
	for _, viewer := range m {
		viewers = append(viewers, viewer.Viewer)
	}
	return viewers
}

// isProtocolSupported returns true the viewer's protocol supports boss bar.
func isProtocolSupported(viewer Viewer) bool {
	// below 1.9 doesn't support boss bars
	p, ok := methods.Protocol(viewer)
	if !ok {
		// assume supported
		return true
	}
	return p.GreaterEqual(version.Minecraft_1_9)
}

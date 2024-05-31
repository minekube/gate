package resourcepack

import (
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/util/uuid"
)

// a legacy (Minecraft 1.17-1.20.2) resource pack handler.
type legacy117Handler struct {
	l *legacyHandler
}

func newLegacy117Handler(player Player, eventMgr event.Manager) *legacy117Handler {
	return &legacy117Handler{
		l: newLegacyHandler(player, eventMgr),
	}
}

var _ Handler = (*legacy117Handler)(nil)

func (h *legacy117Handler) shouldDisconnectForForcePack(event *PlayerResourcePackStatusEvent) bool {
	return h.l.shouldDisconnectForForcePack(event) && !event.OverwriteKick()
}
func (h *legacy117Handler) OnResourcePackResponse(bundle *ResponseBundle) (bool, error) {
	return h.l.onResourcePackResponse(bundle, h.shouldDisconnectForForcePack)
}

func (h *legacy117Handler) FirstAppliedPack() *Info {
	return h.l.FirstAppliedPack()
}

func (h *legacy117Handler) FirstPendingPack() *Info {
	return h.l.FirstPendingPack()
}

func (h *legacy117Handler) AppliedResourcePacks() []*Info {
	return h.l.AppliedResourcePacks()
}

func (h *legacy117Handler) PendingResourcePacks() []*Info {
	return h.l.PendingResourcePacks()
}

func (h *legacy117Handler) ClearAppliedResourcePacks() {
	h.l.ClearAppliedResourcePacks()
}

func (h *legacy117Handler) Remove(id uuid.UUID) bool {
	return h.l.Remove(id)
}

func (h *legacy117Handler) QueueResourcePack(info *Info) error {
	return h.l.QueueResourcePack(info)
}

func (h *legacy117Handler) QueueResourcePackRequest(request *packet.ResourcePackRequest) error {
	return h.l.QueueResourcePackRequest(request)
}

func (h *legacy117Handler) SendResourcePackRequestPacket(pack *Info) error {
	return h.l.SendResourcePackRequestPacket(pack)
}

func (h *legacy117Handler) HandleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool, err error) {
	return h.l.HandleResponseResult(queued, bundle)
}

func (h *legacy117Handler) HasPackAppliedByHash(hash []byte) bool {
	return h.l.HasPackAppliedByHash(hash)
}

func (h *legacy117Handler) CheckAlreadyAppliedPack(hash []byte) error {
	return h.l.CheckAlreadyAppliedPack(hash)
}

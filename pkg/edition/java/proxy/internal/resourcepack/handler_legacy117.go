package resourcepack

import (
	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// a legacy (Minecraft 1.17-1.20.2) resource pack handler.
type legacy117Handler struct {
	l *legacyHandler
}

// newLegacy117Handler creates a legacy handler with the 1.17 force-pack policy.
func newLegacy117Handler(player Player, eventMgr event.Manager) *legacy117Handler {
	return &legacy117Handler{
		l: newLegacyHandler(player, eventMgr),
	}
}

var _ Handler = (*legacy117Handler)(nil)

// shouldDisconnectForForcePack honors a plugin's override of a required-pack kick.
func (h *legacy117Handler) shouldDisconnectForForcePack(event *PlayerResourcePackStatusEvent) bool {
	return h.l.shouldDisconnectForForcePack(event) && !event.OverwriteKick()
}

// OnResourcePackResponse handles a client response with the 1.17 force-pack policy.
func (h *legacy117Handler) OnResourcePackResponse(bundle *ResponseBundle) (bool, error) {
	return h.l.onResourcePackResponse(bundle, h.shouldDisconnectForForcePack)
}

// FirstAppliedPack returns the currently applied legacy pack, if any.
func (h *legacy117Handler) FirstAppliedPack() *Info {
	return h.l.FirstAppliedPack()
}

// FirstPendingPack returns the accepted pack awaiting a terminal response, if any.
func (h *legacy117Handler) FirstPendingPack() *Info {
	return h.l.FirstPendingPack()
}

// AppliedResourcePacks returns the applied pack as a single-element slice, if any.
func (h *legacy117Handler) AppliedResourcePacks() []*Info {
	return h.l.AppliedResourcePacks()
}

// PendingResourcePacks returns the pending pack as a single-element slice, if any.
func (h *legacy117Handler) PendingResourcePacks() []*Info {
	return h.l.PendingResourcePacks()
}

// ClearAppliedResourcePacks clears the recorded applied pack.
func (h *legacy117Handler) ClearAppliedResourcePacks() {
	h.l.ClearAppliedResourcePacks()
}

// Remove panics because legacy clients cannot remove individual resource packs.
func (h *legacy117Handler) Remove(id uuid.UUID) bool {
	return h.l.Remove(id)
}

// QueueResourcePack queues a pack without a retained backend connection.
func (h *legacy117Handler) QueueResourcePack(info *Info) error {
	return h.l.QueueResourcePack(info)
}

// QueueResourcePackFromBackend retains the originating backend for responses.
func (h *legacy117Handler) QueueResourcePackFromBackend(info *Info, backend proto.PacketWriter) error {
	return h.l.QueueResourcePackFromBackend(info, backend)
}

// QueueResourcePackRequest validates and queues a resource pack request.
func (h *legacy117Handler) QueueResourcePackRequest(request *packet.ResourcePackRequest) error {
	return h.l.QueueResourcePackRequest(request)
}

// SendResourcePackRequestPacket writes the pack request to the player.
func (h *legacy117Handler) SendResourcePackRequestPacket(pack *Info) error {
	return h.l.SendResourcePackRequestPacket(pack)
}

// HandleResponseResult handles plugin packs locally and otherwise forwards to the backend in flight.
func (h *legacy117Handler) HandleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool, err error) {
	return h.l.HandleResponseResult(queued, bundle)
}

// HasPackAppliedByHash reports whether a non-nil hash matches the applied pack.
func (h *legacy117Handler) HasPackAppliedByHash(hash []byte) bool {
	return h.l.HasPackAppliedByHash(hash)
}

// CheckAlreadyAppliedPack rejects a hash that matches the applied pack.
func (h *legacy117Handler) CheckAlreadyAppliedPack(hash []byte) error {
	return h.l.CheckAlreadyAppliedPack(hash)
}

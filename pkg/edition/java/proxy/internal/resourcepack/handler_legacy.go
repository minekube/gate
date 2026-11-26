package resourcepack

import (
	"bytes"
	"encoding/hex"
	"errors"

	"github.com/edwingeng/deque/v2"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type legacyResponseState uint8

const (
	legacyResponseUnknown legacyResponseState = iota
	legacyResponseAccepted
	legacyResponseDeclined
)

type legacyQueuedPack struct {
	info    *Info
	backend proto.PacketWriter
}

// Legacy (Minecraft +1.17) ResourcePackHandler.
type legacyHandler struct {
	player   Player
	eventMgr event.Manager

	rwMutex
	prevResourceResponse legacyResponseState
	outstandingPacks     *deque.Deque[*legacyQueuedPack]
	activePack           *legacyQueuedPack
	advancing            bool
	responding           bool
	pendingPack          *Info
	appliedPack          *Info
}

// newLegacyHandler starts with an unknown response so the first pack is prompted;
// later acceptance permits prompts, while a decline automatically rejects optional packs.
func newLegacyHandler(player Player, eventMgr event.Manager) *legacyHandler {
	return &legacyHandler{
		player:               player,
		eventMgr:             eventMgr,
		prevResourceResponse: legacyResponseUnknown,
		outstandingPacks:     deque.NewDeque[*legacyQueuedPack](),
	}
}

var _ Handler = (*legacyHandler)(nil)

// FirstAppliedPack returns the currently applied legacy pack, if any.
func (h *legacyHandler) FirstAppliedPack() *Info {
	h.RLock()
	defer h.RUnlock()
	return h.appliedPack
}

// FirstPendingPack returns the accepted pack awaiting a terminal response, if any.
func (h *legacyHandler) FirstPendingPack() *Info {
	h.RLock()
	defer h.RUnlock()
	return h.pendingPack
}

// AppliedResourcePacks returns the applied pack as a single-element slice, if any.
func (h *legacyHandler) AppliedResourcePacks() []*Info {
	h.RLock()
	defer h.RUnlock()
	if h.appliedPack == nil {
		return nil
	}
	return []*Info{h.appliedPack}
}

// PendingResourcePacks returns the pending pack as a single-element slice, if any.
func (h *legacyHandler) PendingResourcePacks() []*Info {
	h.RLock()
	defer h.RUnlock()
	if h.pendingPack == nil {
		return nil
	}
	return []*Info{h.pendingPack}
}

// ClearAppliedResourcePacks clears the recorded applied pack.
func (h *legacyHandler) ClearAppliedResourcePacks() {
	h.Lock()
	defer h.Unlock()
	// This is valid only for players with 1.20.2 versions
	h.appliedPack = nil
}

// Remove panics because legacy clients cannot remove individual resource packs.
func (h *legacyHandler) Remove(id uuid.UUID) bool {
	panic("Cannot remove a ResourcePack from a legacy client")
}

// QueueResourcePack queues a pack without a retained backend connection.
func (h *legacyHandler) QueueResourcePack(info *Info) error {
	return h.queueResourcePack(info, nil)
}

// QueueResourcePackFromBackend retains the request's connection for responses,
// including automatic declines that have no client packet to forward.
func (h *legacyHandler) QueueResourcePackFromBackend(info *Info, backend proto.PacketWriter) error {
	return h.queueResourcePack(info, backend)
}

// queueResourcePack enqueues a pack and starts one runner when the queue is idle.
// It releases the lock before running the queue so callbacks and writes can reenter.
func (h *legacyHandler) queueResourcePack(info *Info, backend proto.PacketWriter) error {
	if info == nil {
		return errors.New("resource pack info is nil")
	}
	queued := &legacyQueuedPack{info: info, backend: backend}
	h.Lock()
	h.outstandingPacks.PushBack(queued)
	start := h.startQueueLocked()
	h.Unlock()
	if start {
		return h.tickResourcePackQueue(queued)
	}
	return nil
}

// startQueueLocked prevents concurrent callers from starting competing queue runners.
func (h *legacyHandler) startQueueLocked() bool {
	if h.advancing || h.responding || h.activePack != nil || h.outstandingPacks.Len() == 0 {
		return false
	}
	h.advancing = true
	return true
}

// tickResourcePackQueue advances the FIFO with one runner to prevent duplicate
// sends, releasing the lock for callbacks and writes. A non-nil owner identifies
// the waiting caller's request; errors for other requests must not make that caller
// decline the wrong pack.
func (h *legacyHandler) tickResourcePackQueue(owner *legacyQueuedPack) error {
	var result error
	for {
		supportsForced := h.player.Protocol().GreaterEqual(version.Minecraft_1_17)
		h.Lock()
		queued, ok := h.outstandingPacks.Front()
		if !ok || h.activePack != nil || h.responding {
			h.advancing = false
			h.Unlock()
			return result
		}
		decline := h.prevResourceResponse == legacyResponseDeclined &&
			(!queued.info.ShouldForce || !supportsForced)
		if decline {
			// A 1.17+ forced prompt can override an earlier rejection; optional prompts cannot.
			h.outstandingPacks.PopFront()
			h.responding = true
			h.Unlock()
			_, err := h.dispatchResponse(queued, &ResponseBundle{
				ID:     queued.info.ID,
				Hash:   Hash(hex.EncodeToString(queued.info.Hash)),
				Status: DeclinedResponseStatus,
			}, h.shouldDisconnectForForcePack)
			h.Lock()
			h.responding = false
			h.Unlock()
			if owner == nil || owner == queued {
				result = errors.Join(result, err)
			}
			continue
		}

		// Reserve the head before writing so concurrent enqueues cannot send it twice.
		h.activePack = queued
		h.Unlock()
		err := h.SendResourcePackRequestPacket(queued.info)
		if err == nil {
			continue
		}

		h.Lock()
		stillActive := h.activePack == queued
		if stillActive {
			h.outstandingPacks.PopFront()
			h.activePack = nil
			if h.pendingPack == queued.info {
				h.pendingPack = nil
			}
		}
		h.Unlock()
		if !stillActive {
			// A terminal response already completed this request while the write was finishing.
			continue
		}
		if owner != queued && queued.backend != nil {
			// A queued caller has already returned, so it cannot report this send failure.
			err = errors.Join(err, queued.backend.WritePacket((&ResponseBundle{
				ID:     queued.info.ID,
				Hash:   Hash(hex.EncodeToString(queued.info.Hash)),
				Status: DeclinedResponseStatus,
			}).ResponsePacket()))
		}
		if owner == nil || owner == queued {
			result = errors.Join(result, err)
		}
	}
}

// OnResourcePackResponse handles a client response with the legacy force-pack policy.
func (h *legacyHandler) OnResourcePackResponse(bundle *ResponseBundle) (bool, error) {
	return h.onResourcePackResponse(bundle, h.shouldDisconnectForForcePack)
}

// onResourcePackResponse updates the active pack and dispatches its response.
// Terminal responses hold the queue until dispatch finishes, preserving response order
// while callbacks and writes run without the lock and may reenter the handler.
func (h *legacyHandler) onResourcePackResponse(
	bundle *ResponseBundle,
	shouldDisconnectForForcePack func(e *PlayerResourcePackStatusEvent) bool,
) (bool, error) {
	h.Lock()
	queued := h.activePack
	if queued == nil {
		h.Unlock()
		// Without an active request there is no safe backend to forward a late response to.
		return true, nil
	}
	// Legacy responses generally have no ID; use it when available and suppress repeated accepts.
	if (bundle.ID != uuid.Nil && queued.info.ID != uuid.Nil && bundle.ID != queued.info.ID) ||
		(bundle.Status == AcceptedResponseStatus && h.pendingPack == queued.info) {
		h.Unlock()
		return true, nil
	}
	terminal := !bundle.Status.Intermediate()
	if terminal {
		h.outstandingPacks.PopFront()
		h.activePack = nil
		h.pendingPack = nil
		// Keep the next request queued until this response write finishes, even if callbacks enqueue more.
		h.responding = true
	}
	switch bundle.Status {
	case AcceptedResponseStatus:
		h.prevResourceResponse = legacyResponseAccepted
		h.pendingPack = queued.info
	case DeclinedResponseStatus:
		h.prevResourceResponse = legacyResponseDeclined
	case SuccessfulResponseStatus:
		h.appliedPack = queued.info
	case DiscardedResponseStatus:
		if queued.info.ID != uuid.Nil &&
			h.appliedPack != nil &&
			h.appliedPack.ID == queued.info.ID {
			h.appliedPack = nil
		}
	}
	h.Unlock()

	handled, err := h.dispatchResponse(queued, bundle, shouldDisconnectForForcePack)
	if terminal {
		h.Lock()
		h.responding = false
		start := h.startQueueLocked()
		h.Unlock()
		if start {
			err = errors.Join(err, h.tickResourcePackQueue(nil))
		}
	}
	return handled, err
}

// dispatchResponse fires the status event and routes or handles the response.
// It must run without the handler lock because callbacks and writes may reenter.
func (h *legacyHandler) dispatchResponse(
	queued *legacyQueuedPack,
	bundle *ResponseBundle,
	shouldDisconnectForForcePack func(e *PlayerResourcePackStatusEvent) bool,
) (bool, error) {
	// Event callbacks and packet writes may reenter this handler; never invoke them under its lock.
	e := newPlayerResourcePackStatusEvent(h.player, bundle.Status, bundle.ID, *queued.info)
	event.FireParallel(h.eventMgr, e, func(e *PlayerResourcePackStatusEvent) {
		if shouldDisconnectForForcePack(e) {
			h.player.Disconnect(&component.Translation{
				Key: "multiplayer.requiredTexturePrompt.disconnect",
			})
		}
	})
	if queued.backend != nil {
		return true, queued.backend.WritePacket(bundle.ResponsePacket())
	}
	return h.HandleResponseResult(queued.info, bundle)
}

// HasPackAppliedByHash reports whether a non-nil hash matches the applied pack.
func (h *legacyHandler) HasPackAppliedByHash(hash []byte) bool {
	h.RLock()
	defer h.RUnlock()
	if hash == nil {
		return false
	}
	return h.appliedPack != nil && bytes.Equal(h.appliedPack.Hash, hash)
}

// shouldDisconnectForForcePack reports whether a required pack was declined.
func (h *legacyHandler) shouldDisconnectForForcePack(e *PlayerResourcePackStatusEvent) bool {
	return e.Status() == DeclinedResponseStatus &&
		e.PackInfo().ShouldForce
}

// QueueResourcePackRequest validates and queues a resource pack request.
func (h *legacyHandler) QueueResourcePackRequest(request *packet.ResourcePackRequest) error {
	return queueResourcePackRequest(request, h)
}

// SendResourcePackRequestPacket writes the pack request to the player.
func (h *legacyHandler) SendResourcePackRequestPacket(pack *Info) error {
	return sendResourcePackRequestPacket(pack, h.player)
}

// HandleResponseResult handles plugin packs locally and otherwise forwards to the backend in flight.
func (h *legacyHandler) HandleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool, err error) {
	return handleResponseResult(queued, bundle, h.player)
}

// CheckAlreadyAppliedPack rejects a hash that matches the applied pack.
func (h *legacyHandler) CheckAlreadyAppliedPack(hash []byte) error {
	return checkAlreadyAppliedPack(hash, h)
}

package resourcepack

import (
	"bytes"
	"errors"
	"github.com/edwingeng/deque/v2"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Legacy (Minecraft +1.17) ResourcePackHandler.
type legacyHandler struct {
	player   Player
	eventMgr event.Manager

	rwMutex
	prevResourceResponse bool
	outstandingPacks     *deque.Deque[*Info]
	pendingPack          *Info
	appliedPack          *Info
}

func newLegacyHandler(player Player, eventMgr event.Manager) *legacyHandler {
	return &legacyHandler{
		player:           player,
		eventMgr:         eventMgr,
		outstandingPacks: deque.NewDeque[*Info](),
	}
}

var _ Handler = (*legacyHandler)(nil)

func (h *legacyHandler) FirstAppliedPack() *Info {
	h.RLock()
	defer h.RUnlock()
	return h.appliedPack
}

func (h *legacyHandler) FirstPendingPack() *Info {
	h.RLock()
	defer h.RUnlock()
	return h.pendingPack
}

func (h *legacyHandler) AppliedResourcePacks() []*Info {
	h.RLock()
	defer h.RUnlock()
	if h.appliedPack == nil {
		return nil
	}
	return []*Info{h.appliedPack}
}

func (h *legacyHandler) PendingResourcePacks() []*Info {
	h.RLock()
	defer h.RUnlock()
	if h.pendingPack == nil {
		return nil
	}
	return []*Info{h.pendingPack}
}

func (h *legacyHandler) ClearAppliedResourcePacks() {
	h.Lock()
	defer h.Unlock()
	// This is valid only for players with 1.20.2 versions
	h.appliedPack = nil
}

func (h *legacyHandler) Remove(id uuid.UUID) bool {
	panic("Cannot remove a ResourcePack from a legacy client")
}

func (h *legacyHandler) QueueResourcePack(info *Info) error {
	h.Lock()
	defer h.Unlock()
	h.outstandingPacks.PushBack(info)
	if h.outstandingPacks.Len() == 1 {
		return h.tickResourcePackQueue()
	}
	return nil
}

// with comments form java code
func (h *legacyHandler) tickResourcePackQueue() error {
	h.Lock()
	defer h.Unlock()
	queued, ok := h.outstandingPacks.Front()
	if ok {
		// Check if the player declined a resource pack once already
		if !h.prevResourceResponse {
			// If that happened we can flush the queue right away.
			// Unless its 1.17+ and forced it will come back denied anyway
			for h.outstandingPacks.Len() > 0 {
				queued, _ = h.outstandingPacks.Front()
				if queued.ShouldForce && h.player.Protocol().GreaterEqual(version.Minecraft_1_17) {
					break
				}
				resBundle := &ResponseBundle{
					ID:     queued.ID,
					Hash:   queued.Hash,
					Status: DeclinedResponseStatus,
				}
				_, err := h.OnResourcePackResponse(resBundle)
				if err != nil {
					return err
				}
				queued = nil
			}
			if queued == nil {
				// Exit as the queue was cleared
				return nil
			}
		}

		return h.SendResourcePackRequestPacket(queued)
	}

	return nil
}

func (h *legacyHandler) OnResourcePackResponse(bundle *ResponseBundle) (bool, error) {
	return h.onResourcePackResponse(bundle, h.shouldDisconnectForForcePack)
}

func (h *legacyHandler) onResourcePackResponse(
	bundle *ResponseBundle,
	shouldDisconnectForForcePack func(e *PlayerResourcePackStatusEvent) bool,
) (bool, error) {
	h.Lock()
	defer h.Unlock()

	peek := bundle.Status.Intermediate()
	var queued *Info
	if peek {
		queued, _ = h.outstandingPacks.Front()
	} else {
		queued = h.outstandingPacks.PopFront()
	}

	e := newPlayerResourcePackStatusEvent(h.player, bundle.Status, bundle.ID, *queued)
	event.FireParallel(h.eventMgr, e, func(e *PlayerResourcePackStatusEvent) {
		if shouldDisconnectForForcePack(e) {
			h.player.Disconnect(&component.Translation{
				Key: "multiplayer.requiredTexturePrompt.disconnect",
			})
		}
	})

	switch bundle.Status {
	case AcceptedResponseStatus:
		h.prevResourceResponse = true
		h.pendingPack = queued
	case DeclinedResponseStatus:
		h.prevResourceResponse = false
	case SuccessfulResponseStatus:
		h.appliedPack = queued
		h.pendingPack = nil
	case FailedDownloadResponseStatus:
		h.pendingPack = nil
	case DiscardedResponseStatus:
		if queued != nil && queued.ID != uuid.Nil &&
			h.appliedPack != nil &&
			h.appliedPack.ID == queued.ID {
			h.appliedPack = nil
		}
	}

	var err error
	if !peek {
		err = h.tickResourcePackQueue()
	}
	handled, err2 := h.HandleResponseResult(queued, bundle)
	return handled, errors.Join(err, err2)
}

func (h *legacyHandler) HasPackAppliedByHash(hash []byte) bool {
	h.RLock()
	defer h.RUnlock()
	if hash == nil {
		return false
	}
	return h.appliedPack != nil && bytes.Equal(h.appliedPack.Hash, hash)
}

func (h *legacyHandler) shouldDisconnectForForcePack(e *PlayerResourcePackStatusEvent) bool {
	return e.Status() == DeclinedResponseStatus &&
		e.PackInfo().ShouldForce
}

func (h *legacyHandler) QueueResourcePackRequest(request *packet.ResourcePackRequest) error {
	return queueResourcePackRequest(request, h)
}

func (h *legacyHandler) SendResourcePackRequestPacket(pack *Info) error {
	return sendResourcePackRequestPacket(pack, h.player)
}

func (h *legacyHandler) HandleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool, err error) {
	return handleResponseResult(queued, bundle, h.player)
}

func (h *legacyHandler) CheckAlreadyAppliedPack(hash []byte) error {
	return checkAlreadyAppliedPack(hash, h)
}

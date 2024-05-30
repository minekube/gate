package resourcepack

import (
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type (
	HandlerInterface interface {
		FirstAppliedPack() *Info
		FirstPendingPack() *Info
		AppliedResourcePacks() []*Info
		PendingResourcePacks() []*Info
		ClearAppliedResourcePacks()
		Remove(id uuid.UUID) bool
		QueueResourcePack(*Info) error
		// QueueResourcePackRequest queues a resource pack request to be sent to the player.
		// Sends it immediately if the queue is empty.
		QueueResourcePackRequest(*packet.ResourcePackRequest) error
		SendResourcePackRequestPacket(pack *Info) error
		OnResourcePackResponse(bundle *ResponseBundle)
		handleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool)
		HasPackAppliedByHash(hash []byte) bool
		CheckAlreadyAppliedPack(hash []byte) error
	}
	// Player is a player that can receive resource packs.
	Player interface {
		// PacketWriter returns the writer to the player connection.
		proto.PacketWriter
		BundleHandler() *BundleDelimiterHandler
		State() states.State
		// Protocol returns the protocol of the player.
		Protocol() proto.Protocol
		// Backend returns the writer to the backend server connection the player is connected to, if any.
		Backend() proto.PacketWriter
	}
	baseHandler struct {
		parent HandlerInterface
		player Player
	}
	LegacyResourcePackHandler struct {
		*baseHandler
	}
	Legacy117ResourcePackHandler struct {
		*baseHandler
	}
)

func NewHandler(player Player) HandlerInterface {
	if player.Protocol().Lower(version.Minecraft_1_17) {
		parent := &LegacyResourcePackHandler{}
		parent.baseHandler = &baseHandler{parent: parent}
		return parent
	}
	if player.Protocol().Lower(version.Minecraft_1_20_3) {
		parent := &Legacy117ResourcePackHandler{}
		parent.baseHandler = &baseHandler{parent: parent}
		return parent
	}
	return newModernHandler(player)
}

func queueResourcePackRequest(request *packet.ResourcePackRequest, next HandlerInterface) error {
	info, err := InfoForRequest(request)
	if err != nil {
		return err
	}
	if next != nil {
		if err = next.CheckAlreadyAppliedPack(info.Hash); err != nil {
			return err
		}
		return next.QueueResourcePack(info)
	}
	return nil
}

func (h *baseHandler) SendResourcePackRequestPacket(queued *Info) error {
	return h.player.WritePacket(queued.RequestPacket(h.player.Protocol()))
}

func (h *baseHandler) handleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool) {
	// If Gate, through a plugin, has sent a resource pack to the client,
	// there is no need to report the status of the response to the server
	// since it has no information that a resource pack has been sent
	handled = queued != nil && queued.Origin == PluginOnProxyOrigin
	if !handled {
		backend := h.player.Backend()
		if backend != nil {
			_ = backend.WritePacket(bundle.ResponsePacket())
			return true
		}
	}
	return handled
}

//
//

func (p *connectedPlayer) queueResourcePack(info ResourcePackInfo) error {
	if info.URL == "" {
		return errors.New("missing resource-pack url")
	}
	if len(info.Hash) > 0 && len(info.Hash) != 20 {
		return errors.New("resource-pack hash length must be 20")
	}
	p.mu.Lock()
	p.outstandingResourcePacks.PushBack(&info)
	size := p.outstandingResourcePacks.Len()
	p.mu.Unlock()
	if size == 1 {
		return p.tickResourcePackQueue()
	}
	return nil
}

func (p *connectedPlayer) tickResourcePackQueue() error {
	p.mu.RLock()
	if p.outstandingResourcePacks.Len() == 0 {
		p.mu.RUnlock()
		return nil
	}
	queued := p.outstandingResourcePacks.Front()
	previousResourceResponse := p.previousResourceResponse
	p.mu.RUnlock()

	// Check if the player declined a resource pack once already
	if previousResourceResponse != nil && !*previousResourceResponse {
		// If that happened we can flush the queue right away.
		// Unless its 1.17+ and forced it will come back denied anyway
		for {
			p.mu.Lock()
			if p.outstandingResourcePacks.Len() != 0 {
				p.mu.Unlock()
				break
			}
			queued = p.outstandingResourcePacks.Front()
			p.mu.Unlock()
			if queued.ShouldForce && p.Protocol().GreaterEqual(version.Minecraft_1_17) {
				break
			}
			_ = p.onResourcePackResponse(packet.DeclinedResourcePackResponseStatus)
			queued = nil
		}
		if queued == nil {
			// Exit as the queue was cleared
			return nil
		}
	}

	return p.WritePacket(queued.RequestPacket(p.Protocol()))
}


// Processes a client response to a sent resource-pack.
func (p *connectedPlayer) onResourcePackResponse(status ResourcePackResponseStatus) bool {
	peek := status.Intermediate()

	p.mu.Lock()
	if p.outstandingResourcePacks.Len() == 0 {
		p.mu.Unlock()
		return false
	}

	var queued *ResourcePackInfo
	if peek {
		queued = p.outstandingResourcePacks.Front()
	} else {
		queued = p.outstandingResourcePacks.PopFront()
	}
	p.mu.Unlock()

	e := newPlayerResourcePackStatusEvent(
		p, status,, *queued, false,
	)
	p.eventMgr.Fire(e)

	if e.Status() == DeclinedResourcePackResponseStatus &&
		e.PackInfo().ShouldForce &&
		(!e.OverwriteKick() || e.Player().Protocol().GreaterEqual(version.Minecraft_1_17)) {
		e.Player().Disconnect(&component.Translation{
			Key: "multiplayer.requiredTexturePrompt.disconnect",
		})
	}

	p.mu.Lock()
	switch status {
	case AcceptedResourcePackResponseStatus:
		b := true
		p.previousResourceResponse = &b
		p.pendingResourcePack = queued
	case DeclinedResourcePackResponseStatus:
		b := false
		p.previousResourceResponse = &b
	case SuccessfulResourcePackResponseStatus:
		p.appliedResourcePack = queued
		p.previousResourceResponse = nil
		p.pendingResourcePack = nil
	case FailedDownloadResourcePackResponseStatus:
		p.pendingResourcePack = nil
	}
	p.mu.Unlock()

	if !peek {
		_ = p.tickResourcePackQueue()
	}
	return queued != nil && queued.Origin != DownstreamServerResourcePackOrigin
}
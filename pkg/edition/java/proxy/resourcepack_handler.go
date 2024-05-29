package proxy

import (
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/util/uuid"
)

type (
	resourcePackHandlerInterface interface {
		FirstAppliedPack() *ResourcePackInfo
		FirstPendingPack() *ResourcePackInfo
		AppliedResourcePacks() []*ResourcePackInfo
		PendingResourcePacks() []*ResourcePackInfo
		ClearAppliedResourcePacks()
		Remove(id uuid.UUID) bool
		QueueResourcePack(pack *ResourcePackInfo)
		QueueResourcePackRequest(pack *ResourcePackInfo)
		SendResourcePackRequestPacket(pack *ResourcePackInfo) error
		OnResourcePackResponse(bundle *ResourcePackResponseBundle)
		HandleResponseResult(queued *ResourcePackInfo, bundle *ResourcePackResponseBundle)
		HasPackAppliedByHash(hash []byte) bool
		CheckAlreadyAppliedPack(hash []byte) bool
	}
	resourcePackHandler struct {
		player *connectedPlayer
	}
	legacyResourcePackHandler struct {
		*resourcePackHandler
	}
	legacy117ResourcePackHandler struct {
		*resourcePackHandler
	}
	modernResourcePackHandler struct {
		*resourcePackHandler
	}
)

func newResourcePackHandler(player *connectedPlayer) resourcePackHandlerInterface {
	protocol := player.Protocol()
	handler := &resourcePackHandler{player: player}
	if protocol.Lower(version.Minecraft_1_17) {
		return &legacyResourcePackHandler{resourcePackHandler: handler}
	}
	if protocol.Lower(version.Minecraft_1_20_3) {
		return &legacy117ResourcePackHandler{resourcePackHandler: handler}
	}
	return &modernResourcePackHandler{resourcePackHandler: handler}
}

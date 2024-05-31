package resourcepack

import (
	"errors"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/state"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

type (
	// Handler is an internal interface implemented by
	// resource pack handlers of different protocol versions.
	Handler interface {
		FirstAppliedPack() *Info
		FirstPendingPack() *Info
		AppliedResourcePacks() []*Info
		PendingResourcePacks() []*Info
		// ClearAppliedResourcePacks clears the applied resource pack field.
		ClearAppliedResourcePacks()
		Remove(id uuid.UUID) bool
		// QueueResourcePack queues a resource-pack for sending to
		// the player and sends it immediately if the queue is empty.
		QueueResourcePack(*Info) error
		// QueueResourcePackRequest queues a resource pack request to be sent to the player.
		// Sends it immediately if the queue is empty.
		QueueResourcePackRequest(*packet.ResourcePackRequest) error
		SendResourcePackRequestPacket(pack *Info) error
		// OnResourcePackResponse processes a client response to a sent resource-pack.
		// No action will be taken in the following cases:
		//
		//  - DOWNLOADED: The resource pack is downloaded and will be applied to the client,
		// 	no action is required in Velocity.
		// 	- INVALID_URL: The client has received a resource pack request
		// 	and the first check it performs is if the URL is valid, if not,
		// 	it will return this value.
		// 	- FAILED_RELOAD: When trying to reload the client's resources,
		// 	an error occurred while reloading a resource pack.
		// 	- DECLINED: Only in modern versions, as the resource pack has already been rejected,
		// 	there is nothing to do, if the resource pack is required,
		// 	the client will be kicked out of the server.
		OnResourcePackResponse(bundle *ResponseBundle) (bool, error)
		HandleResponseResult(queued *Info, bundle *ResponseBundle) (handled bool, err error)
		// HasPackAppliedByHash checks if a resource pack with the given hash has already been applied.
		HasPackAppliedByHash(hash []byte) bool
		CheckAlreadyAppliedPack(hash []byte) error
	}
	// Player is an internal player interface consumed by resource pack handlers
	Player interface {
		ID() uuid.UUID
		// PacketWriter returns the writer to the player connection.
		proto.PacketWriter
		BundleHandler() *BundleDelimiterHandler
		State() *state.Registry
		// Protocol returns the protocol of the player.
		Protocol() proto.Protocol
		// Backend returns the writer to the backend server connection the player is connected to, if any.
		Backend() proto.PacketWriter
		Disconnect(reason component.Component)
	}
)

// NewHandler creates a new resource pack handler appropriate for the player's protocol version.
func NewHandler(player Player) Handler {
	if player.Protocol().Lower(version.Minecraft_1_17) {
		return newLegacyHandler(player)
	}
	if player.Protocol().Lower(version.Minecraft_1_20_3) {
		return newLegacy117Handler(player)
	}
	return newModernHandler(player)
}

func queueResourcePackRequest(request *packet.ResourcePackRequest, dep interface {
	CheckAlreadyAppliedPack(hash []byte) error
	QueueResourcePack(*Info) error
}) error {
	info, err := InfoForRequest(request)
	if err != nil {
		return err
	}
	if err = dep.CheckAlreadyAppliedPack(info.Hash); err != nil {
		return err
	}
	return dep.QueueResourcePack(info)
}

func sendResourcePackRequestPacket(queued *Info, player Player) error {
	if queued == nil {
		return nil
	}
	return player.WritePacket(queued.RequestPacket(player.Protocol()))
}

func handleResponseResult(queued *Info, bundle *ResponseBundle, player Player) (handled bool, err error) {
	// If Gate, through a plugin, has sent a resource pack to the client,
	// there is no need to report the status of the response to the server
	// since it has no information that a resource pack has been sent
	handled = queued != nil && queued.Origin == PluginOnProxyOrigin
	if !handled {
		backend := player.Backend()
		if backend != nil {
			return handled, backend.WritePacket(bundle.ResponsePacket())
		}
	}
	return handled, nil
}

func checkAlreadyAppliedPack(hash []byte, dep interface {
	HasPackAppliedByHash(hash []byte) bool
}) error {
	if dep.HasPackAppliedByHash(hash) {
		return errors.New("cannot apply a resource pack already applied")
	}
	return nil
}

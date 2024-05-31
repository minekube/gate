package resourcepack

import (
	"go.minekube.com/gate/pkg/edition/java/proto/packet"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
	"go.minekube.com/gate/pkg/gate/proto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// ResponseStatus represents the possible statuses for the resource pack.
type ResponseStatus = packet.ResponseStatus

// Possible statuses for a resource pack.
const (
	// SuccessfulResponseStatus indicates the resource pack was applied successfully.
	SuccessfulResponseStatus ResponseStatus = packet.SuccessfulResourcePackResponseStatus
	// DeclinedResponseStatus indicates the player declined to download the resource pack.
	DeclinedResponseStatus ResponseStatus = packet.DeclinedResourcePackResponseStatus
	// FailedDownloadResponseStatus indicates the player could not download the resource pack.
	FailedDownloadResponseStatus ResponseStatus = packet.FailedDownloadResourcePackResponseStatus
	// AcceptedResponseStatus indicates the player has accepted the resource pack and is now downloading it.
	AcceptedResponseStatus ResponseStatus = packet.AcceptedResourcePackResponseStatus
	// DownloadedResponseStatus indicates the player has downloaded the resource pack.
	DownloadedResponseStatus ResponseStatus = packet.DownloadedResourcePackResponseStatus
	// InvalidURLResponseStatus indicates the URL of the resource pack failed to load.
	InvalidURLResponseStatus ResponseStatus = packet.InvalidURLResourcePackResponseStatus
	// FailedToReloadResponseStatus indicates the player failed to reload the resource pack.
	FailedToReloadResponseStatus ResponseStatus = packet.FailedToReloadResourcePackResponseStatus
	// DiscardedResponseStatus indicates the resource pack was discarded.
	DiscardedResponseStatus ResponseStatus = packet.DiscardedResourcePackResponseStatus
)

// PlayerResourcePackStatusEvent is fired when the status of a resource pack sent to the player by the server is
// changed. Depending on the result of this event (which the proxy will wait until completely fired),
// the player may be kicked from the server.
type PlayerResourcePackStatusEvent struct {
	player        player
	status        ResponseStatus
	packID        uuid.UUID
	packInfo      Info
	overwriteKick bool
}

type player interface {
	ID() uuid.UUID
	Protocol() proto.Protocol
}

func newPlayerResourcePackStatusEvent(
	player player,
	status ResponseStatus,
	packID uuid.UUID,
	packInfo Info,
) *PlayerResourcePackStatusEvent {
	if packID == uuid.Nil {
		packID = packInfo.ID
	}
	return &PlayerResourcePackStatusEvent{
		player:   player,
		status:   status,
		packID:   packID,
		packInfo: packInfo,
	}
}

// PlayerID returns the id of the player affected by the change in resource pack status.
// To get the player, use the Proxy.Player method.
func (p *PlayerResourcePackStatusEvent) PlayerID() uuid.UUID {
	return p.player.ID()
}

// Status returns the new status for the resource pack.
func (p *PlayerResourcePackStatusEvent) Status() ResponseStatus {
	return p.status
}

// PackInfo returns the ResourcePackInfo this response is for.
func (p *PlayerResourcePackStatusEvent) PackInfo() Info {
	return p.packInfo
}

// OverwriteKick returns whether to override the kick resulting from ResourcePackInfo.ShouldForce() being true.
func (p *PlayerResourcePackStatusEvent) OverwriteKick() bool {
	return p.overwriteKick
}

// SetOverwriteKick can set to true to prevent ResourcePackInfo.ShouldForce()
// from kicking the player. Overwriting this kick is only possible on versions older than 1.17,
// as the client or server will enforce this regardless. Cancelling the resulting
// kick-events will not prevent the player from disconnecting from the proxy.
func (p *PlayerResourcePackStatusEvent) SetOverwriteKick(overwriteKick bool) {
	if p.player.Protocol().LowerEqual(version.Minecraft_1_17) {
		return // overwriteKick is not supported on 1.17 or newer
	}
	p.overwriteKick = overwriteKick
}

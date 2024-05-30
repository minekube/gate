package player

import (
	guuid "github.com/google/uuid"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// ChatSession represents a chat session helf by a player.
type ChatSession interface {
	SessionID() uuid.UUID // The ID of this chat session.
	crypto.KeyIdentifiable
}

// AsHoverEntity returns a HoverEvent that shows the player as an entity.
// The result can be passed to component.ShowEntity() to create a hover event for chat messages.
func AsHoverEntity(player interface {
	ID() uuid.UUID
	Username() string
}) *component.ShowEntityHoverType {
	return &component.ShowEntityHoverType{
		Id:   guuid.UUID(player.ID()),
		Name: &component.Text{Content: player.Username()},
	}
}

package player

import (
	"go.minekube.com/gate/pkg/edition/java/proxy/crypto"
	"go.minekube.com/gate/pkg/util/uuid"
)

// ChatSession represents a chat session helf by a player.
type ChatSession interface {
	SessionID() uuid.UUID // The ID of this chat session.
	crypto.KeyIdentifiable
}

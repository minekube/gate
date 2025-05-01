package cookie

import (
	"context"

	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

type Cookie interface {
	// Sends a store packet to the player. The payload has a maximum size of 5 kiB
	Store(proxy.Player) error

	Key() key.Key
	SetKey(key.Key)

	Payload() []byte
	SetPayload([]byte)
}

// Creates a new cookie, that can be stored
func New(
	key key.Key,
	payload []byte,
) Cookie {
	return &cookie{
		key:     key,
		payload: payload,
	}
}

// Sends a request packet to the player. In return, the player will send the stored cookie, which can be listened to in the CookieResponseEvent.
func Request(p proxy.Player, key key.Key) error {
	return request(p, key)
}

// Sends a request packet to the player. Instead of listening for the stored cookie in the CookieResponseEvent, it will listen in this function.
func RequestWithResult(p proxy.Player, key key.Key, ctx context.Context) (Cookie, error) {
	return requestWithResult(p, key, ctx)
}

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

func New(
	key key.Key,
	payload []byte,
) Cookie {
	return &cookie{
		key:     key,
		payload: payload,
	}
}

// Sends a request packet to the player. In return, the player will send the stored cookie, which can be listened to in the CookieResponseEvent event.
func Request(p proxy.Player, key key.Key) error {
	return request(p, key)
}

func RequestWithResult(p proxy.Player, key key.Key, ctx context.Context) ([]byte, error) {
	return requestWithResult(p, key, ctx)
}

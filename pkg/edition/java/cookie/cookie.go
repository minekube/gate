package cookie

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
	"go.minekube.com/gate/pkg/edition/java/proto/state/states"
	"go.minekube.com/gate/pkg/gate/proto"
)

var (
	ErrUnsupportedClientProtocol = errors.New("player version must be at least 1.20.5 to use cookies")
	ErrUnsupportedState          = fmt.Errorf("cookie can only be stored in %s or %s state", states.ConfigState, states.PlayState)
)

const (
	MaxPayloadSize        = cookie.MaxPayloadSize
	DefaultRequestTimeout = 10 * time.Second
)

// Cookie is cookie that can be sent and received from clients.
//
// Use Store to save a cookie on the client.
// When the cookie is modified, Store must be called again to save the changes.
//
// The maximum size of a cookie is 5 kiB.
type Cookie struct {
	Key     key.Key
	Payload []byte
}

// Client is a player that can store and send stored cookies.
type Client interface {
	proto.PacketWriter
	Context() context.Context
}

// Store stores a cookie on the player's client.
//
// If the player's protocol is below 1.20.5, ErrUnsupportedClientProtocol is returned.
// If the player's state is not states.ConfigState or states.PlayState, ErrUnsupportedState is returned.
func Store(c Client, cookie *Cookie) error {
	return store(c, cookie)
}

// Clear clears a stored cookie from the player's client.
// This is a helper function that stores a cookie with an empty payload.
//
// If the player's protocol is below 1.20.5, ErrUnsupportedClientProtocol is returned.
// If the player's state is not states.ConfigState or states.PlayState, ErrUnsupportedState is returned.
func Clear(c Client, key key.Key) error {
	return store(c, &Cookie{Key: key, Payload: nil})
}

// Request requests a stored cookie from the player's client by a given key.
// This is a blocking operation and the context should be used to timeout the request.
// The DefaultRequestTimeout always applies as a safety net.
//
// The event manager is used to listen for the cookie response event fired by Gate.
// Use proxy.Event() to get the proxy's event manager.
//
// Request only subscribes to the cookie response event only until Request returns.
func Request(ctx context.Context, c Client, key key.Key, eventMgr event.Manager) (*Cookie, error) {
	return request(ctx, c, key, eventMgr)
}

// RequestAndForget works like Request but does not wait for a response.
//
// The cookie response event will still be fired by Gate and can be listened to in the event manager.
func RequestAndForget(c Client, key key.Key) error {
	return requestAndForget(c, key)
}


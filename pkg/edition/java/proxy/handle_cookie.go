package proxy

import (
	"sync"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
)

type requestListener struct {
	Mu      sync.Mutex
	Pending map[string]chan []byte
}

var RequestListenerPerPlayer map[Player]*requestListener

func handleCookieResponse(cr *cookie.CookieResponse, p Player, eventMgr event.Manager) {
	// ensure map exists
	if RequestListenerPerPlayer == nil {
		RequestListenerPerPlayer = make(map[Player]*requestListener)
	}

	// create a requestListener if player doesn't have one
	r, ok := RequestListenerPerPlayer[p]
	if !ok {
		r = &requestListener{
			Pending: make(map[string]chan []byte),
		}
		RequestListenerPerPlayer[p] = r
	}

	r.Mu.Lock()
	responseChan, ok := r.Pending[cr.Key.String()]
	r.Mu.Unlock()

	// Check if the cookie.RequestWithResult() is waiting for this packet, otherwise fire the event.
	if ok {
		responseChan <- cr.Payload
	} else {
		event.FireParallel(eventMgr, newCookieResponseEvent(p, cr.Key, cr.Payload))
	}
}

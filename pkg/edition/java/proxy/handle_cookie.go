package proxy

import (
	"sync"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
)

type cookieRequestListener struct {
	Mu      sync.Mutex
	Pending map[string]chan []byte
}

var CookieRequestListenerPlayerMap map[Player]*cookieRequestListener

func handleCookieResponse(cr *cookie.CookieResponse, p Player, eventMgr event.Manager) {
	// ensure map exists
	if CookieRequestListenerPlayerMap == nil {
		CookieRequestListenerPlayerMap = make(map[Player]*cookieRequestListener)
	}

	// create a cookieRequestListener if player doesn't have one
	r, ok := CookieRequestListenerPlayerMap[p]
	if !ok {
		r = &cookieRequestListener{
			Pending: make(map[string]chan []byte),
		}
		CookieRequestListenerPlayerMap[p] = r
	}

	r.Mu.Lock()
	responseChan, ok := r.Pending[cr.Key.String()]
	r.Mu.Unlock()

	// Check if the cookie.RequestWithResult is waiting for this packet, otherwise fire the event.
	if ok {
		responseChan <- cr.Payload
	} else {
		event.FireParallel(eventMgr, newCookieResponseEvent(p, cr.Key, cr.Payload))
	}
}

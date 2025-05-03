package proxy

import (
	"sync"

	"github.com/robinbraemer/event"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/cookie"
)

var CookieRequestListenerPlayerMap map[Player]*sync.Map

func handleCookieResponse(cr *cookie.CookieResponse, p Player, eventMgr event.Manager) {
	// ensure map exists
	if CookieRequestListenerPlayerMap == nil {
		CookieRequestListenerPlayerMap = make(map[Player]*sync.Map)
	}

	// create a cookieRequestListener if player doesn't have one
	r, ok := CookieRequestListenerPlayerMap[p]
	if !ok {
		r = &sync.Map{}
		CookieRequestListenerPlayerMap[p] = r
	}

	// Check if the cookie.RequestWithResult is waiting for this packet, otherwise fire the event.
	value, ok := r.Load(cr.Key.String())
	if !ok {
		event.FireParallel(eventMgr, newCookieResponseEvent(p, cr.Key, cr.Payload))
	}

	responseChan, isChan := value.(chan []byte)
	if isChan {
		responseChan <- cr.Payload
	}
}

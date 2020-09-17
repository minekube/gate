package proxy

import "errors"

var (
	ErrProxyAlreadyStarted = errors.New("proxy was already started")
)

type Proxy interface {
	// Start starts the proxy and blocks until stop is closed or a proxy has an error starting.
	Start(stopCh <-chan struct{}) error
}

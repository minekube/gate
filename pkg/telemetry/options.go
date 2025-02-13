package telemetry

import "github.com/robinbraemer/event"

// Options contains configuration options for telemetry initialization.
type Options struct {
	// EventMgr is the optional event manager to use.
	EventMgr event.Manager
}
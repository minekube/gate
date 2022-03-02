package process

import (
	"time"

	"go.minekube.com/gate/pkg/runtime/logr"
)

// Collection is a runtime utility to manage a collection of processes
// and handle graceful shutdown if one of them errors at any time.
type Collection interface {
	// Add will cause the Runnable to be started when Start is called.
	// It is started automatically if the Collection was already started.
	Add(Runnable) error
	// Start starts all registered Runnables and blocks until the stop channel is closed.
	// Returns an error if there is an error starting any Runnable.
	Start(stop <-chan struct{}) error
}

// Runnable allows a component to be started.
// It's very important that Start blocks until it's done running.
type Runnable interface {
	// Start starts running the component. The component will stop running
	// when the channel is closed. Start blocks until the channel is closed or
	// an error occurs.
	Start(stop <-chan struct{}) error
}

// RunnableFunc implements Runnable using a function.
// It's very important that it blocks until it's done running.
type RunnableFunc func(stop <-chan struct{}) error

// Start implements Runnable.
func (r RunnableFunc) Start(stop <-chan struct{}) error { return r(stop) }

// Options are the arguments for creating a new Collection
type Options struct {
	// The logger that should be used by the Collection.
	// If none is set, no logging is done.
	Logger logr.Logger
	// Whether all Runnables in the Collection should be running or none.
	// The complete Collection will be stopped if one Runnable returns.
	AllOrNothing bool
	// GracefulShutdownTimeout is the duration given to Runnable to
	// stop before the Collection actually returns on stop.
	// To disable graceful shutdown, set to time.Duration(0)
	// To use graceful shutdown without timeout, set to a negative duration, e.g. time.Duration(-1).
	// If not set DefaultGracefulShutdownPeriod is used.
	GracefulShutdownTimeout *time.Duration
}

// DefaultGracefulShutdownPeriod is the default graceful shutdown
// timeout to wait for Runnables to shutdown on Collection stop.
const DefaultGracefulShutdownPeriod = 30 * time.Second

// New returns a new Collection for managing Runnables.
func New(options Options, runnables ...Runnable) Collection {
	// Set default values for options fields
	options = setOptionsDefaults(options)

	coll := &collection{
		internalStop:            make(chan struct{}),
		log:                     options.Logger,
		allOrNothing:            options.AllOrNothing,
		gracefulShutdownTimeout: *options.GracefulShutdownTimeout,
	}
	for _, r := range runnables {
		_ = coll.Add(r)
	}
	return coll
}

// setOptionsDefaults set default values for Options fields
func setOptionsDefaults(options Options) Options {
	if options.GracefulShutdownTimeout == nil {
		gracefulShutdownTimeout := DefaultGracefulShutdownPeriod
		options.GracefulShutdownTimeout = &gracefulShutdownTimeout
	}
	if options.Logger == nil {
		options.Logger = logr.NopLog
	}
	return options
}

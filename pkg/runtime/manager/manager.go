package manager

import (
	"go.minekube.com/gate/pkg/event"
	"go.minekube.com/gate/pkg/runtime/logr"
	"time"
)

// Manager initializes shared dependencies such as Caches and Clients, and provides them to Runnables.
// A Manager is required to create Proxies.
type Manager interface {
	// Add will set requested dependencies on the Runnable, and cause the Runnable to be
	// started when Start is called. Add will inject any dependencies for which the argument
	// implements an inject interface - e.g. inject.Logger.
	Add(Runnable) error
	// Start starts all registered Runnables and blocks until the stop channel is closed.
	// Returns an error if there is an error starting any Runnable.
	Start(stop <-chan struct{}) error
	// Logger returns this manager's logger.
	Logger() logr.Logger
	// Event returns the manager's event manager.
	Event() event.Manager
	// SetFields will set any dependencies on an object for which the object has implemented an inject
	// interface - e.g. inject.Logger.
	SetFields(interface{}) error
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
// It's very important that the given function block
// until it's done running.
type RunnableFunc func(<-chan struct{}) error

// Start implements Runnable
func (r RunnableFunc) Start(s <-chan struct{}) error {
	return r(s)
}

// Options are the arguments for creating a new Manager
type Options struct {
	// Logger is the logger that should be used by this manager.
	// If none is set, it defaults to logr.Log global logger.
	Logger logr.Logger
	// Event is the event manager that should be used by this manager.
	// If none is set, a new one is creates.
	Event event.Manager
	// GracefulShutdownTimeout is the duration given to runnable to
	// stop before the manager actually returns on stop.
	// To disable graceful shutdown, set to time.Duration(0)
	// To use graceful shutdown without timeout, set to a negative duration, e.g. time.Duration(-1).
	// If note set DefaultGracefulShutdownPeriod is used.
	GracefulShutdownTimeout *time.Duration
}

const DefaultGracefulShutdownPeriod = 30 * time.Second

// New returns a new Manager for creating proxies.
func New(options Options) (Manager, error) {
	// Set default values for options fields
	options = setOptionsDefaults(options)

	return &proxyManager{
		internalStop:            make(chan struct{}),
		logger:                  options.Logger,
		gracefulShutdownTimeout: *options.GracefulShutdownTimeout,
		event:                   options.Event,
	}, nil
}

// setOptionsDefaults set default values for Options fields
func setOptionsDefaults(options Options) Options {
	if options.GracefulShutdownTimeout == nil {
		gracefulShutdownTimeout := DefaultGracefulShutdownPeriod
		options.GracefulShutdownTimeout = &gracefulShutdownTimeout
	}

	if options.Logger == nil {
		options.Logger = logr.Log
	}
	if options.Event == nil {
		options.Event = event.New(options.Logger.WithName("event"))
	}

	return options
}

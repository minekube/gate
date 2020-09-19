package inject

import "go.minekube.com/gate/pkg/runtime/logr"

// Stoppable is used by the Manager to inject stop channel.
// It is used for gracefully stopping components in run by Manager.
type Stoppable interface {
	InjectStopChannel(<-chan struct{}) error
}

// StopChannelInto will set stop channel on i and return the result if it implements Stoppable.
// Returns false if i does not implement Stoppable.
func StopChannelInto(stop <-chan struct{}, i interface{}) (bool, error) {
	if s, ok := i.(Stoppable); ok {
		return true, s.InjectStopChannel(stop)
	}
	return false, nil
}

// Logger is used to inject Loggers into components that need them
// and don't otherwise have opinions.
type Logger interface {
	InjectLogger(l logr.Logger) error
}

// LoggerInto will set the logger on the given object if it implements inject.Logger,
// returning true if a InjectLogger was called, and false otherwise.
func LoggerInto(l logr.Logger, i interface{}) (bool, error) {
	if injectable, wantsLogger := i.(Logger); wantsLogger {
		return true, injectable.InjectLogger(l)
	}
	return false, nil
}

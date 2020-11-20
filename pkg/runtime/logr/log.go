package logr

import (
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Logger is Gate's logger interface.
type Logger interface {
	logr.Logger
}

// SetLogger sets a concrete logging implementation for all deferred Loggers.
func SetLogger(l Logger) {
	Log.Fulfill(l)
}

// Log is the base logger used.
// It delegates to another logr.Logger.
// You *must* call SetLogger to get any actual logging.
var Log = log.NewDelegatingLogger(NullLog)

// NullLog is a Logger that does nothing.
var NullLog Logger = log.NullLogger{}

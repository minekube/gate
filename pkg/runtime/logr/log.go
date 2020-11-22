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
	dLog.Fulfill(l)
}

// Log is the base logger used.
// It delegates to another logr.Logger.
// You *must* call SetLogger to get any actual logging.
var Log Logger = dLog
var dLog = log.NewDelegatingLogger(NopLog)

// NopLog is a Logger that does nothing.
var NopLog Logger = log.NullLogger{}

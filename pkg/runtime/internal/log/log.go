package log

import (
	"go.minekube.com/gate/pkg/runtime/logr"
)

// RuntimeLog is a base parent logger for use inside proxy-runtime.
var RuntimeLog = logr.Log.WithName("proxy-runtime")

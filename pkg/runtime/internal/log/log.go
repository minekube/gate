package log

import "go.minekube.com/gate/pkg/runtime/log"

// RuntimeLog is a base parent logger for use inside proxy-runtime.
var RuntimeLog = log.Log.WithName("proxy-runtime")

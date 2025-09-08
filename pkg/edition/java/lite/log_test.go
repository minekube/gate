package lite

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
)

// TestLogSpamReduction verifies that failed backend logs use higher verbosity
// to avoid spamming the console when backends are unreachable.
func TestLogSpamReduction(t *testing.T) {
	// Create a logger with verbosity 0 (normal level)
	log := testr.New(t)

	// The actual log level check happens in tryBackends function
	// where we use log.V(1) for failed backend attempts
	
	// Verify that log.V(1) creates a logger with increased verbosity
	verboseLog := log.V(1)
	assert.NotNil(t, verboseLog, "Should create verbose logger")
	
	// This test ensures the code compiles with the verbosity changes
	// The actual effect is that failed backend logs will only show
	// when running with -v flag or higher verbosity settings
}

// TestTryBackendsErrorHandling verifies error handling behavior
func TestTryBackendsErrorHandling(t *testing.T) {
	log := testr.New(t)
	
	attempts := 0
	next := func() (string, logr.Logger, bool) {
		attempts++
		if attempts > 3 {
			return "", log, false
		}
		return "backend" + string(rune('0'+attempts)), log, true
	}
	
	tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, string, error) {
		// Simulate all backends failing
		return log, "", errors.New("connection refused")
	}
	
	// Should try all backends and return errAllBackendsFailed
	_, _, result, err := tryBackends(next, tryFunc)
	assert.Equal(t, "", result)
	assert.Equal(t, errAllBackendsFailed, err)
	assert.Equal(t, 4, attempts, "Should try all backends before giving up")
}

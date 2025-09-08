package lite

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
)

// TestBackendSelection_TriesAllBackends verifies that when one backend fails,
// the system tries other available backends in sequential order (fixes issue #2)
func TestBackendSelection_TriesAllBackends(t *testing.T) {
	log := testr.New(t)

	// Create a simple nextBackend function that simulates the real behavior
	backends := []string{"backend1:25565", "backend2:25565", "backend3:25565"}
	remainingBackends := make([]string, len(backends))
	copy(remainingBackends, backends)

	nextBackend := func() (string, logr.Logger, bool) {
		if len(remainingBackends) == 0 {
			return "", log, false
		}
		// Pop first backend - simple sequential approach
		backend := remainingBackends[0]
		remainingBackends = remainingBackends[1:]
		return backend, log, true
	}

	// Track which backends were tried
	triedBackends := make([]string, 0, 3)
	attemptCount := 0

	// Try function that simulates all backends failing
	tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, string, error) {
		attemptCount++
		triedBackends = append(triedBackends, backendAddr)
		// Simulate all backends failing
		return log, "", errors.New("connection refused")
	}

	// Should try all backends before giving up
	_, _, _, err := tryBackends(nextBackend, tryFunc)

	assert.Equal(t, errAllBackendsFailed, err, "Should return errAllBackendsFailed when all backends fail")
	assert.Equal(t, 3, attemptCount, "Should try all 3 backends")
	assert.Equal(t, 3, len(triedBackends), "Should have tried 3 different backends")

	// Verify backends were tried in sequential order
	assert.Equal(t, []string{"backend1:25565", "backend2:25565", "backend3:25565"}, triedBackends,
		"Should try backends in sequential order")
}

// TestBackendSelection_SucceedsOnSecondBackend verifies that if the first backend fails
// but the second succeeds, the connection is established with the second backend
func TestBackendSelection_SucceedsOnSecondBackend(t *testing.T) {
	log := testr.New(t)

	// Create simple nextBackend function with sequential order
	backends := []string{"bad:25565", "good:25565", "another:25565"}
	remainingBackends := make([]string, len(backends))
	copy(remainingBackends, backends)

	nextBackend := func() (string, logr.Logger, bool) {
		if len(remainingBackends) == 0 {
			return "", log, false
		}
		// Pop first backend - simple sequential approach
		backend := remainingBackends[0]
		remainingBackends = remainingBackends[1:]
		return backend, log, true
	}

	// Track attempts
	attemptCount := 0

	// Try function where first backend fails, second succeeds
	tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, string, error) {
		attemptCount++
		if backendAddr == "bad:25565" {
			return log, "", errors.New("connection refused")
		}
		// Second backend succeeds
		return log, "success", nil
	}

	// Should succeed with the second backend
	backendAddr, _, result, err := tryBackends(nextBackend, tryFunc)

	assert.NoError(t, err, "Should succeed when second backend is reachable")
	assert.Equal(t, "success", result, "Should return success from second backend")
	assert.Equal(t, "good:25565", backendAddr, "Should connect to the good backend")
	assert.Equal(t, 2, attemptCount, "Should try 2 backends (first fails, second succeeds)")
}

// TestBackendSelection_NoDuplicateAttempts verifies that the same backend is not tried twice
func TestBackendSelection_NoDuplicateAttempts(t *testing.T) {
	log := testr.New(t)

	// Create simple nextBackend with sequential order
	backends := []string{"backend1:25565", "backend2:25565"}
	remainingBackends := make([]string, len(backends))
	copy(remainingBackends, backends)

	nextBackend := func() (string, logr.Logger, bool) {
		if len(remainingBackends) == 0 {
			return "", log, false
		}
		// Pop first backend - guarantees no duplicates
		backend := remainingBackends[0]
		remainingBackends = remainingBackends[1:]
		return backend, log, true
	}

	// Track which backends were tried
	backendAttempts := make(map[string]int)

	// Try function that tracks attempts
	tryFunc := func(log logr.Logger, backendAddr string) (logr.Logger, string, error) {
		backendAttempts[backendAddr]++
		return log, "", errors.New("connection refused")
	}

	// Try all backends
	_, _, _, err := tryBackends(nextBackend, tryFunc)

	assert.Equal(t, errAllBackendsFailed, err)

	// Each backend should only be tried once (guaranteed by pop-first approach)
	for backend, count := range backendAttempts {
		assert.Equal(t, 1, count, "Backend %s should only be tried once, got %d attempts", backend, count)
	}

	// Should have tried both backends
	assert.Equal(t, 2, len(backendAttempts), "Should have tried exactly 2 backends")
}

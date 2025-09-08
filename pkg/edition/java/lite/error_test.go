package lite

import (
	"fmt"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.minekube.com/gate/pkg/util/errs"
)

func TestRealErrorVerbosity(t *testing.T) {
	// Test with the actual error format from your logs
	realError := fmt.Errorf("failed to dial route: failed to connect to backend localhost:25566: %w",
		fmt.Errorf("dial tcp [::1]:25566: connect: %w", syscall.ECONNREFUSED))

	// Test our detection
	isRefused := IsConnectionRefused(realError)
	assert.True(t, isRefused, "Should detect connection refused in real error format")

	// Test what verbosity this gets
	verbosityError := &errs.VerbosityError{
		Verbosity: 1, // This should be 1 for connection refused
		Err:       realError,
	}

	t.Logf("Real error: %v", realError)
	t.Logf("Is connection refused: %v", isRefused)
	t.Logf("Would get verbosity: %v", verbosityError.Verbosity)
}

func TestDialRouteErrorFormat(t *testing.T) {
	// Simulate what dialRoute does for connection refused
	backendAddr := "localhost:25566"
	baseErr := syscall.ECONNREFUSED

	// What dialRoute creates
	dialErr := fmt.Errorf("failed to connect to backend %s: %w", backendAddr, baseErr)

	// Test detection on this format
	isRefused := IsConnectionRefused(dialErr)
	assert.True(t, isRefused, "Should detect connection refused in dialRoute error format")

	// Test detection on the syscall error directly
	isRefusedDirect := IsConnectionRefused(baseErr)
	assert.True(t, isRefusedDirect, "Should detect ECONNREFUSED directly")
}

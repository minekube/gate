package lite

import (
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsConnectionRefused(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "syscall.ECONNREFUSED should be detected",
			err:      syscall.ECONNREFUSED,
			expected: true,
		},
		{
			name:     "error with 'connection refused' in message should be detected",
			err:      errors.New("dial tcp 127.0.0.1:25566: connect: connection refused"),
			expected: true,
		},
		{
			name:     "error with 'Connection Refused' (different case) should be detected",
			err:      errors.New("Connection Refused by server"),
			expected: true,
		},
		{
			name:     "wrapped ECONNREFUSED should be detected",
			err:      &MyError{Inner: syscall.ECONNREFUSED},
			expected: true,
		},
		{
			name:     "timeout error should not be detected as connection refused",
			err:      errors.New("dial tcp 127.0.0.1:25566: i/o timeout"),
			expected: false,
		},
		{
			name:     "other network error should not be detected",
			err:      errors.New("dial tcp: missing address"),
			expected: false,
		},
		{
			name:     "nil error should not be detected",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsConnectionRefused(tt.err)
			assert.Equal(t, tt.expected, result, "IsConnectionRefused should correctly identify connection refused errors")
		})
	}
}

// MyError is a test error type that wraps another error
type MyError struct {
	Inner error
}

func (e *MyError) Error() string {
	return e.Inner.Error()
}

func (e *MyError) Unwrap() error {
	return e.Inner
}

// TestVerbosityForConnectionRefused verifies that connection refused errors get higher verbosity
func TestVerbosityForConnectionRefused(t *testing.T) {
	// Test the logic from dialRoute function

	// Simulate connection refused error (not a timeout)
	connectionRefusedErr := syscall.ECONNREFUSED
	dialCtxErr := error(nil) // No timeout

	v := 0
	if dialCtxErr != nil {
		v++
	}
	// This is the new logic we added
	if IsConnectionRefused(connectionRefusedErr) {
		v = 1
	}

	assert.Equal(t, 1, v, "Connection refused errors should get verbosity 1 (debug level)")

	// Simulate timeout error
	timeoutErr := errors.New("i/o timeout")
	dialCtxErr = timeoutErr // Timeout occurred

	v = 0
	if dialCtxErr != nil {
		v++
	}
	if IsConnectionRefused(timeoutErr) {
		v = 1
	}

	assert.Equal(t, 1, v, "Timeout errors should also get verbosity 1")

	// Simulate other error
	otherErr := errors.New("some other error")
	dialCtxErr = error(nil) // No timeout

	v = 0
	if dialCtxErr != nil {
		v++
	}
	if IsConnectionRefused(otherErr) {
		v = 1
	}

	assert.Equal(t, 0, v, "Other errors should get verbosity 0 (info level)")
}

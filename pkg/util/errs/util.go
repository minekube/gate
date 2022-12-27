package errs

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/go-logr/logr"
)

var (
	ErrMissingConfig = errors.New("config is missing")
)

// SilentError is an error wrapper type that silences an
// error and only logs them in the debug log.
//
// It is usually used to prevent spamming the default
// log when Minecraft clients send invalid packets which cannot be read.
type SilentError struct{ Err error }

func (e *SilentError) Error() string {
	return e.Err.Error()
}

func NewSilentErr(format string, a ...any) error {
	return &SilentError{Err: fmt.Errorf(format, a...)}
}

func WrapSilent(wrappedErr error) error {
	return &SilentError{wrappedErr}
}

func (e *SilentError) Unwrap() error { return e.Err }

// IsConnClosedErr returns true if err indicates a closed connection error.
func IsConnClosedErr(err error) bool {
	return err != nil && (errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET))
}

// VerbosityError is an error wrapper that specifies the log verbosity of the wrapped error.
type VerbosityError struct {
	Err       error
	Verbosity int
}

// V returns a new Logger instance with the specific verbosity level specified if the error is a VerbosityError.
// See logr.Logger#V().
func V(log logr.Logger, err error) logr.Logger {
	var v *VerbosityError
	if errors.As(err, &v) {
		return log.V(v.Verbosity)
	}
	return log
}

func (e *VerbosityError) Error() string {
	return e.Err.Error()
}
func (e *VerbosityError) Unwrap() error { return e.Err }

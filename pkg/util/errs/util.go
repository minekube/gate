package errs

import (
	"errors"
	"fmt"
	"net"
	"syscall"
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

func NewSilentErr(format string, a ...interface{}) error {
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

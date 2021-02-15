package errs

import (
	"errors"
	"fmt"
)

var (
	ErrMissingConfig = errors.New("config is missing")
)

// SilentError is an error wrapper type that silences an
// error and only logs them in the debug log.
//
// It is usually used to prevent spamming the default
// log when Minecraft clients send invalid packets which cannot be read.
type SilentError struct{ error }

func (e *SilentError) Error() string {
	return e.error.Error()
}

func NewSilentErr(format string, a ...interface{}) error {
	return &SilentError{fmt.Errorf(format, a...)}
}

func WrapSilent(wrappedErr error) error {
	return &SilentError{wrappedErr}
}

func (e *SilentError) Unwrap() error { return e.error }

// see https://github.com/golang/go/issues/4373 for details
func IsConnClosedErr(err error) bool {
	return err != nil &&
		(err.Error() == "use of closed network connection" ||
			err.Error() == "read: connection reset by peer")
}

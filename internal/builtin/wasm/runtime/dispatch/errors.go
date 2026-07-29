package dispatch

import (
	"errors"
	"fmt"
)

type ErrorKind string

const (
	ErrorHostPanic          ErrorKind = "host-panic"
	ErrorUnknownOperation   ErrorKind = "unknown-operation"
	ErrorDuplicateOperation ErrorKind = "duplicate-operation"
	ErrorArgumentCount      ErrorKind = "argument-count"
	ErrorArgumentType       ErrorKind = "argument-type"
	ErrorInvalidCallable    ErrorKind = "invalid-callable"
)

type Error struct {
	Kind      ErrorKind
	Operation Operation
	Detail    string
	Cause     error
}

func (err *Error) Error() string {
	message := "wasm dispatch " + string(err.Kind)
	if err.Operation.Identity != "" {
		message += " in " + err.Operation.Identity
	}
	if err.Detail != "" {
		message += ": " + err.Detail
	}
	if err.Cause != nil {
		message += ": " + err.Cause.Error()
	}
	return message
}

func (err *Error) Unwrap() error {
	return err.Cause
}

func (err *Error) Is(target error) bool {
	var dispatchError *Error
	return errors.As(target, &dispatchError) && err.Kind == dispatchError.Kind
}

var (
	ErrHostPanic          = &Error{Kind: ErrorHostPanic}
	ErrUnknownOperation   = &Error{Kind: ErrorUnknownOperation}
	ErrDuplicateOperation = &Error{Kind: ErrorDuplicateOperation}
	ErrArgumentCount      = &Error{Kind: ErrorArgumentCount}
	ErrArgumentType       = &Error{Kind: ErrorArgumentType}
	ErrInvalidCallable    = &Error{Kind: ErrorInvalidCallable}
)

func operationError(operation Operation, cause error) error {
	return fmt.Errorf("%s: %w", operation.Identity, cause)
}

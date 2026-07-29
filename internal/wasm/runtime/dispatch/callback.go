package dispatch

import (
	"errors"
	"fmt"
	"reflect"

	"go.minekube.com/gate/internal/wasm/runtime/resources"
	"go.minekube.com/gate/internal/wasm/runtime/wire"
)

// CallbackInvoker enters a guest callback export by its generated type and
// guest-assigned identity.
type CallbackInvoker interface {
	InvokeCallback(callbackTypeID uint32, guestID uint64, input []byte) ([]byte, error)
}

// GuestCallback is the language-neutral callback identity stored behind a
// generated WIT callback resource.
type GuestCallback struct {
	TypeID  uint32
	GuestID uint64
	Invoker CallbackInvoker
}

func bindGuestCallback(
	callback GuestCallback,
	expected reflect.Type,
	table *resources.Table,
) (reflect.Value, error) {
	if expected.Kind() != reflect.Func {
		return reflect.Value{}, fmt.Errorf("guest callback requires a function, got %s", expected)
	}
	if callback.TypeID == 0 || callback.Invoker == nil {
		return reflect.Value{}, errors.New("guest callback identity is incomplete")
	}
	return reflect.MakeFunc(expected, func(arguments []reflect.Value) []reflect.Value {
		values := make([]any, len(arguments))
		for index, argument := range arguments {
			values[index] = argument.Interface()
		}
		scope, err := table.BeginScope(resources.LifetimeBorrowedCall)
		if err != nil {
			return callbackFailure(expected, err)
		}
		defer func() { _ = scope.Close() }()
		marshaled, err := wire.MarshalGoValuesBorrowed(values, table, scope)
		if err != nil {
			return callbackFailure(expected, err)
		}
		input, err := wire.Encode(marshaled)
		if err != nil {
			return callbackFailure(expected, err)
		}
		output, err := callback.Invoker.InvokeCallback(
			callback.TypeID,
			callback.GuestID,
			input,
		)
		if err != nil {
			return callbackFailure(expected, err)
		}
		response, err := wire.DecodeResponse(output)
		if err != nil {
			return callbackFailure(expected, err)
		}
		if response.Error != nil {
			return callbackFailure(expected, fmt.Errorf(
				"%s: %s (%s)",
				response.Error.Operation,
				response.Error.Message,
				response.Error.Kind,
			))
		}
		return callbackSuccess(expected, response.Values, table)
	}), nil
}

func callbackSuccess(
	expected reflect.Type,
	values []any,
	table *resources.Table,
) []reflect.Value {
	resultCount, _ := callbackResultShape(expected)
	if len(values) != resultCount {
		return callbackFailure(
			expected,
			fmt.Errorf("guest callback returned %d values, want %d", len(values), resultCount),
		)
	}
	results := zeroCallbackResults(expected)
	for index, value := range values {
		converted, err := convertAnyValue(value, expected.Out(index), table)
		if err != nil {
			return callbackFailure(
				expected,
				fmt.Errorf("guest callback result %d: %w", index, err),
			)
		}
		results[index] = converted
	}
	return results
}

func callbackFailure(expected reflect.Type, failure error) []reflect.Value {
	_, hasError := callbackResultShape(expected)
	if !hasError {
		panic(failure)
	}
	results := zeroCallbackResults(expected)
	errorValue := reflect.ValueOf(failure)
	errorIndex := expected.NumOut() - 1
	if errorValue.Type().AssignableTo(expected.Out(errorIndex)) {
		results[errorIndex] = errorValue
		return results
	}
	if errorValue.Type().ConvertibleTo(expected.Out(errorIndex)) {
		results[errorIndex] = errorValue.Convert(expected.Out(errorIndex))
		return results
	}
	panic(failure)
}

func callbackResultShape(expected reflect.Type) (count int, hasError bool) {
	count = expected.NumOut()
	if count > 0 && expected.Out(count-1).Implements(errorType) {
		return count - 1, true
	}
	return count, false
}

func zeroCallbackResults(expected reflect.Type) []reflect.Value {
	results := make([]reflect.Value, expected.NumOut())
	for index := range results {
		results[index] = reflect.Zero(expected.Out(index))
	}
	return results
}

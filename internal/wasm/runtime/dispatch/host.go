package dispatch

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"go.minekube.com/gate/internal/wasm/runtime/resources"
)

type OperationID uint32

type Handler func(context.Context, *Host, []any) ([]any, error)

type Operation struct {
	ID       OperationID
	Identity string
	Handler  Handler
}

type Host struct {
	mu         sync.RWMutex
	resources  *resources.Table
	operations map[OperationID]Operation
	closed     bool
}

func NewHost(table *resources.Table) *Host {
	return &Host{
		resources:  table,
		operations: make(map[OperationID]Operation),
	}
}

func (host *Host) Register(operation Operation) error {
	host.mu.Lock()
	defer host.mu.Unlock()
	if _, exists := host.operations[operation.ID]; exists ||
		operation.ID == 0 ||
		operation.Identity == "" ||
		operation.Handler == nil {
		return &Error{
			Kind: ErrorDuplicateOperation, Operation: operation,
			Detail: "operation ID or identity is invalid or already registered",
		}
	}
	host.operations[operation.ID] = operation
	return nil
}

func (host *Host) Invoke(
	ctx context.Context,
	id OperationID,
	arguments []any,
) (results []any, err error) {
	host.mu.RLock()
	operation, exists := host.operations[id]
	closed := host.closed
	host.mu.RUnlock()
	if closed || !exists {
		return nil, &Error{
			Kind:      ErrorUnknownOperation,
			Operation: Operation{ID: id},
		}
	}
	defer recoverPanic(operation, &err)
	return operation.Handler(ctx, host, arguments)
}

func (host *Host) Call(
	_ context.Context,
	operation Operation,
	callable any,
	arguments []any,
	variadic bool,
) (results []any, err error) {
	defer recoverPanic(operation, &err)
	function := reflect.ValueOf(callable)
	if !function.IsValid() || function.Kind() != reflect.Func {
		return nil, &Error{
			Kind: ErrorInvalidCallable, Operation: operation,
			Detail: fmt.Sprintf("%T is not callable", callable),
		}
	}
	values, err := callArguments(operation, function.Type(), arguments, variadic)
	if err != nil {
		return nil, err
	}
	var returned []reflect.Value
	if variadic {
		returned = function.CallSlice(values)
	} else {
		returned = function.Call(values)
	}
	return unpackResults(operation, returned)
}

func (host *Host) CallMethod(
	ctx context.Context,
	operation Operation,
	handle resources.Handle,
	typeIdentity string,
	method string,
	arguments []any,
	variadic bool,
) ([]any, error) {
	receiver, err := host.resources.Resolve(handle, typeIdentity)
	if err != nil {
		return nil, operationError(operation, err)
	}
	value := reflect.ValueOf(receiver)
	if !value.IsValid() {
		return nil, &Error{
			Kind: ErrorInvalidCallable, Operation: operation,
			Detail: "resource receiver is nil",
		}
	}
	callable := value.MethodByName(method)
	if !callable.IsValid() {
		return nil, &Error{
			Kind: ErrorInvalidCallable, Operation: operation,
			Detail: fmt.Sprintf("receiver %T has no method %s", receiver, method),
		}
	}
	return host.Call(ctx, operation, callable.Interface(), arguments, variadic)
}

func (host *Host) CallResourceMethod(
	ctx context.Context,
	operation Operation,
	typeIdentity string,
	method string,
	arguments []any,
	variadic bool,
) ([]any, error) {
	if len(arguments) == 0 {
		return nil, host.ArgumentCount(operation, 0, 1)
	}
	handleType := reflect.TypeFor[resources.Handle]()
	handleValue, err := argumentValue(operation, 0, arguments[0], handleType)
	if err != nil {
		return nil, err
	}
	return host.CallMethod(
		ctx,
		operation,
		handleValue.Interface().(resources.Handle),
		typeIdentity,
		method,
		arguments[1:],
		variadic,
	)
}

func (host *Host) ArgumentCount(
	operation Operation,
	got int,
	want int,
) error {
	return &Error{
		Kind: ErrorArgumentCount, Operation: operation,
		Detail: fmt.Sprintf("got %d, want %d", got, want),
	}
}

func (host *Host) Assign(
	operation Operation,
	target any,
	value any,
) (err error) {
	defer recoverPanic(operation, &err)
	pointer := reflect.ValueOf(target)
	if !pointer.IsValid() || pointer.Kind() != reflect.Pointer ||
		pointer.IsNil() {
		return &Error{
			Kind: ErrorArgumentType, Operation: operation,
			Detail: "assignment target is not a non-nil pointer",
		}
	}
	converted, err := argumentValue(operation, 0, value, pointer.Elem().Type())
	if err != nil {
		return err
	}
	pointer.Elem().Set(converted)
	return nil
}

func (host *Host) Resources() *resources.Table {
	return host.resources
}

func (host *Host) Close() error {
	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		return nil
	}
	host.closed = true
	host.mu.Unlock()
	if host.resources == nil {
		return nil
	}
	return host.resources.Close()
}

func callArguments(
	operation Operation,
	function reflect.Type,
	arguments []any,
	variadic bool,
) ([]reflect.Value, error) {
	if variadic {
		if len(arguments) != function.NumIn() {
			return nil, &Error{
				Kind: ErrorArgumentCount, Operation: operation,
				Detail: fmt.Sprintf("got %d, want %d including variadic slice", len(arguments), function.NumIn()),
			}
		}
	} else if len(arguments) != function.NumIn() {
		return nil, &Error{
			Kind: ErrorArgumentCount, Operation: operation,
			Detail: fmt.Sprintf("got %d, want %d", len(arguments), function.NumIn()),
		}
	}
	values := make([]reflect.Value, len(arguments))
	for index, argument := range arguments {
		expected := function.In(index)
		converted, err := argumentValue(operation, index, argument, expected)
		if err != nil {
			return nil, err
		}
		values[index] = converted
	}
	return values, nil
}

func argumentValue(
	operation Operation,
	index int,
	argument any,
	expected reflect.Type,
) (reflect.Value, error) {
	if argument == nil {
		switch expected.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
			reflect.Pointer, reflect.Slice:
			return reflect.Zero(expected), nil
		default:
			return reflect.Value{}, &Error{
				Kind: ErrorArgumentType, Operation: operation,
				Detail: fmt.Sprintf("argument %d is nil, want %s", index, expected),
			}
		}
	}
	value := reflect.ValueOf(argument)
	if value.Type().AssignableTo(expected) {
		return value, nil
	}
	if value.Type().ConvertibleTo(expected) {
		return value.Convert(expected), nil
	}
	return reflect.Value{}, &Error{
		Kind: ErrorArgumentType, Operation: operation,
		Detail: fmt.Sprintf(
			"argument %d has type %s, want %s",
			index,
			value.Type(),
			expected,
		),
	}
}

var errorType = reflect.TypeFor[error]()

func unpackResults(
	operation Operation,
	returned []reflect.Value,
) ([]any, error) {
	if len(returned) != 0 {
		last := returned[len(returned)-1]
		if last.Type().Implements(errorType) {
			returned = returned[:len(returned)-1]
			if !last.IsNil() {
				return nil, operationError(operation, last.Interface().(error))
			}
		}
	}
	results := make([]any, len(returned))
	for index, value := range returned {
		results[index] = value.Interface()
	}
	return results, nil
}

func recoverPanic(operation Operation, err *error) {
	if recovered := recover(); recovered != nil {
		*err = &Error{
			Kind: ErrorHostPanic, Operation: operation,
			Detail: fmt.Sprint(recovered),
		}
	}
}

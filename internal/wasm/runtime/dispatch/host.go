package dispatch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"go.minekube.com/gate/internal/wasm/runtime/resources"
	"go.minekube.com/gate/internal/wasm/runtime/wire"
)

type OperationID uint32

type Handler func(context.Context, *Host, []any) ([]any, error)
type ExtensionHandler func(context.Context, Operation, []any) ([]any, error)

type Operation struct {
	ID       OperationID
	Identity string
	Handler  Handler
}

type Host struct {
	mu         sync.RWMutex
	resources  *resources.Table
	operations map[OperationID]Operation
	extension  ExtensionHandler
	closed     bool
}

func (host *Host) SetExtension(handler ExtensionHandler) error {
	if handler == nil {
		return errors.New("dispatch extension handler is required")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.closed {
		return errors.New("dispatch host is closed")
	}
	if host.extension != nil {
		return errors.New("dispatch extension handler is already set")
	}
	host.extension = handler
	return nil
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
	values, err := callArguments(
		operation,
		function.Type(),
		arguments,
		variadic,
		host.resources,
	)
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

// CallExtension converts generated arguments using a statically generated Go
// function signature, then delegates the operation to the runtime extension.
func (host *Host) CallExtension(
	ctx context.Context,
	operation Operation,
	signature any,
	arguments []any,
) ([]any, error) {
	function := reflect.ValueOf(signature)
	if !function.IsValid() || function.Kind() != reflect.Func {
		return nil, &Error{
			Kind: ErrorInvalidCallable, Operation: operation,
			Detail: "extension signature is not callable",
		}
	}
	values, err := callArguments(
		operation,
		function.Type(),
		arguments,
		false,
		host.resources,
	)
	if err != nil {
		return nil, err
	}
	converted := make([]any, len(values))
	for index, value := range values {
		converted[index] = value.Interface()
	}
	host.mu.RLock()
	extension := host.extension
	host.mu.RUnlock()
	if extension == nil {
		return nil, &Error{
			Kind: ErrorInvalidCallable, Operation: operation,
			Detail: "runtime extension handler is unavailable",
		}
	}
	return extension(ctx, operation, converted)
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
	if errorsIsResourceTypeMismatch(err) {
		receiver, _, err = host.resources.ResolveAny(handle)
	}
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
	handleValue, err := argumentValue(
		operation,
		0,
		arguments[0],
		handleType,
		host.resources,
	)
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

// CallResource invokes a function value stored in the resource table. The
// first argument is the callback resource and the remaining values are the
// function arguments.
func (host *Host) CallResource(
	ctx context.Context,
	operation Operation,
	arguments []any,
) ([]any, error) {
	if len(arguments) == 0 {
		return nil, host.ArgumentCount(operation, 0, 1)
	}
	handleValue, err := argumentValue(
		operation,
		0,
		arguments[0],
		reflect.TypeFor[resources.Handle](),
		host.resources,
	)
	if err != nil {
		return nil, err
	}
	value, _, err := host.resources.ResolveAny(
		handleValue.Interface().(resources.Handle),
	)
	if err != nil {
		return nil, operationError(operation, err)
	}
	return host.Call(ctx, operation, value, arguments[1:], false)
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
	converted, err := argumentValue(
		operation,
		0,
		value,
		pointer.Elem().Type(),
		host.resources,
	)
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
	table *resources.Table,
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
		converted, err := argumentValue(
			operation,
			index,
			argument,
			expected,
			table,
		)
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
	table *resources.Table,
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
	converted, err := convertWireValue(argument, expected, table)
	if err == nil {
		return converted, nil
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

func convertWireValue(
	argument any,
	expected reflect.Type,
	table *resources.Table,
) (reflect.Value, error) {
	if resource, ok := argument.(wire.Resource); ok {
		if table == nil {
			return reflect.Value{}, fmt.Errorf("resource table is unavailable")
		}
		resolved, _, err := table.ResolveAny(resources.Handle(resource))
		if err != nil {
			return reflect.Value{}, err
		}
		return convertResolvedValue(reflect.ValueOf(resolved), expected, table)
	}
	if expected.Kind() == reflect.Pointer {
		record, ok := argument.(wire.Record)
		if !ok {
			return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
		}
		value := reflect.New(expected.Elem())
		if err := populateRecord(value.Elem(), record, table); err != nil {
			return reflect.Value{}, err
		}
		return value, nil
	}
	switch expected.Kind() {
	case reflect.Struct:
		record, ok := argument.(wire.Record)
		if !ok {
			return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
		}
		value := reflect.New(expected).Elem()
		if err := populateRecord(value, record, table); err != nil {
			return reflect.Value{}, err
		}
		return value, nil
	case reflect.Slice:
		items, ok := argument.([]any)
		if !ok {
			return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
		}
		value := reflect.MakeSlice(expected, len(items), len(items))
		for index, item := range items {
			converted, err := convertAnyValue(item, expected.Elem(), table)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("item %d: %w", index, err)
			}
			value.Index(index).Set(converted)
		}
		return value, nil
	case reflect.Array:
		items, ok := argument.([]any)
		if !ok || len(items) != expected.Len() {
			return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
		}
		value := reflect.New(expected).Elem()
		for index, item := range items {
			converted, err := convertAnyValue(item, expected.Elem(), table)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("item %d: %w", index, err)
			}
			value.Index(index).Set(converted)
		}
		return value, nil
	case reflect.Map:
		entries, ok := argument.(wire.Map)
		if !ok {
			return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
		}
		value := reflect.MakeMapWithSize(expected, len(entries))
		for index, entry := range entries {
			key, err := convertAnyValue(entry.Key, expected.Key(), table)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("map key %d: %w", index, err)
			}
			item, err := convertAnyValue(entry.Value, expected.Elem(), table)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("map value %d: %w", index, err)
			}
			value.SetMapIndex(key, item)
		}
		return value, nil
	case reflect.Complex64, reflect.Complex128:
		record, ok := argument.(wire.Record)
		if !ok {
			return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
		}
		var realPart, imaginaryPart float64
		for _, field := range record {
			part := reflect.ValueOf(field.Value)
			if !part.IsValid() || !part.Type().ConvertibleTo(reflect.TypeFor[float64]()) {
				continue
			}
			switch field.Name {
			case "real":
				realPart = part.Convert(reflect.TypeFor[float64]()).Float()
			case "imaginary":
				imaginaryPart = part.Convert(reflect.TypeFor[float64]()).Float()
			}
		}
		value := reflect.New(expected).Elem()
		value.SetComplex(complex(realPart, imaginaryPart))
		return value, nil
	}
	return reflect.Value{}, fmt.Errorf("cannot convert %T to %s", argument, expected)
}

func convertAnyValue(
	argument any,
	expected reflect.Type,
	table *resources.Table,
) (reflect.Value, error) {
	if argument == nil {
		switch expected.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
			reflect.Pointer, reflect.Slice:
			return reflect.Zero(expected), nil
		default:
			return reflect.Value{}, fmt.Errorf("nil is not valid for %s", expected)
		}
	}
	value := reflect.ValueOf(argument)
	if value.Type().AssignableTo(expected) {
		return value, nil
	}
	if value.Type().ConvertibleTo(expected) {
		return value.Convert(expected), nil
	}
	return convertWireValue(argument, expected, table)
}

func convertResolvedValue(
	value reflect.Value,
	expected reflect.Type,
	table *resources.Table,
) (reflect.Value, error) {
	if !value.IsValid() {
		return reflect.Zero(expected), nil
	}
	if value.CanInterface() {
		if callback, ok := value.Interface().(GuestCallback); ok {
			return bindGuestCallback(callback, expected, table)
		}
	}
	if value.Type().AssignableTo(expected) {
		return value, nil
	}
	if value.Type().ConvertibleTo(expected) {
		return value.Convert(expected), nil
	}
	if expected.Kind() == reflect.Interface && value.Type().Implements(expected) {
		return value, nil
	}
	return reflect.Value{}, fmt.Errorf(
		"resource Go type %s is not assignable to %s",
		value.Type(),
		expected,
	)
}

func populateRecord(
	target reflect.Value,
	record wire.Record,
	table *resources.Table,
) error {
	fields := make(map[string]any, len(record))
	for _, field := range record {
		fields[normalizeFieldName(field.Name)] = field.Value
	}
	typ := target.Type()
	for index := 0; index < target.NumField(); index++ {
		field := typ.Field(index)
		if !field.IsExported() {
			continue
		}
		input, exists := fields[normalizeFieldName(field.Name)]
		if !exists {
			continue
		}
		converted, err := convertAnyValue(input, field.Type, table)
		if err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}
		target.Field(index).Set(converted)
	}
	return nil
}

func normalizeFieldName(name string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return -1
	}, name)
}

func errorsIsResourceTypeMismatch(err error) bool {
	var resourceError *resources.Error
	return err != nil &&
		errors.As(err, &resourceError) &&
		resourceError.Kind == resources.ErrorTypeMismatch
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

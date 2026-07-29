// Package gatehost adapts the generated Gate API dispatch table to the native
// Wasmtime host callback.
package gatehost

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/brigodier"

	"go.minekube.com/gate/internal/builtin/wasm/generated"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/dispatch"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/resources"
	"go.minekube.com/gate/internal/builtin/wasm/runtime/wire"
	"go.minekube.com/gate/internal/wasm/runtime/native"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
)

const (
	contextTypeIdentity = "context.Context"
	proxyTypeIdentity   = "go.minekube.com/gate/pkg/edition/java/proxy.Proxy"
)

type Host struct {
	mu             sync.RWMutex
	context        context.Context
	table          *resources.Table
	dispatch       *dispatch.Host
	callbackRoot   native.CallbackInvoker
	callbackActive native.CallbackInvoker
	eventManager   event.Manager
	commandManager *command.Manager
	subscriptions  []func()
	timers         map[*ownedTimer]struct{}
	timerLimit     uint32
	closed         bool
	stopOnce       sync.Once
	plugin         string
	contextHandle  resources.Handle
	proxyHandle    resources.Handle
}

func New(
	plugin string,
	ctx context.Context,
	gateProxy *proxy.Proxy,
	resourceCapacity uint32,
) (*Host, error) {
	if ctx == nil {
		return nil, errors.New("wasm plugin context is required")
	}
	if gateProxy == nil {
		return nil, errors.New("wasm plugin proxy is required")
	}
	table := resources.NewTable(plugin, resourceCapacity)
	host := &Host{
		plugin:         plugin,
		context:        ctx,
		table:          table,
		dispatch:       dispatch.NewHost(table),
		eventManager:   gateProxy.Event(),
		commandManager: gateProxy.Command(),
		timers:         make(map[*ownedTimer]struct{}),
		timerLimit:     1024,
	}
	var err error
	host.contextHandle, err = table.Insert(
		ctx,
		contextTypeIdentity,
		resources.LifetimePlugin,
		nil,
	)
	if err != nil {
		_ = host.Close()
		return nil, err
	}
	host.proxyHandle, err = table.Insert(
		gateProxy,
		proxyTypeIdentity,
		resources.LifetimePlugin,
		nil,
	)
	if err != nil {
		_ = host.Close()
		return nil, err
	}
	if err := api.RegisterGeneratedOperations(host.dispatch); err != nil {
		_ = host.Close()
		return nil, fmt.Errorf("register generated Gate API: %w", err)
	}
	if err := api.RegisterGeneratedCallbacks(host.dispatch); err != nil {
		_ = host.Close()
		return nil, fmt.Errorf("register generated Gate callbacks: %w", err)
	}
	if err := host.dispatch.SetExtension(host.invokeExtension); err != nil {
		_ = host.Close()
		return nil, fmt.Errorf("register Gate wasm runtime extensions: %w", err)
	}
	return host, nil
}

func (host *Host) ContextHandle() uint64 {
	return uint64(host.contextHandle)
}

func (host *Host) ProxyHandle() uint64 {
	return uint64(host.proxyHandle)
}

func (host *Host) SetTimerLimit(limit uint32) error {
	if limit == 0 {
		return errors.New("wasm plugin timer limit must be greater than zero")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.timers) != 0 {
		return errors.New("wasm plugin timer limit cannot change after scheduling")
	}
	host.timerLimit = limit
	return nil
}

func (host *Host) Invoke(operationID uint32, input []byte) ([]byte, error) {
	return host.invoke(operationID, input)
}

func (host *Host) InvokeWithReentry(
	reentry native.CallbackInvoker,
	operationID uint32,
	input []byte,
) ([]byte, error) {
	if reentry == nil {
		return nil, errors.New("wasm callback reentry is required")
	}
	host.mu.Lock()
	previous := host.callbackActive
	host.callbackActive = reentry
	host.mu.Unlock()
	defer func() {
		host.mu.Lock()
		host.callbackActive = previous
		host.mu.Unlock()
	}()
	return host.invoke(operationID, input)
}

func (host *Host) invoke(operationID uint32, input []byte) ([]byte, error) {
	arguments, err := wire.Decode(input)
	if err != nil {
		return nil, fmt.Errorf("decode operation %d arguments: %w", operationID, err)
	}
	results, err := host.dispatch.Invoke(
		host.context,
		dispatch.OperationID(operationID),
		arguments,
	)
	if err != nil {
		return encodeDispatchError(operationID, err)
	}
	values, err := wire.MarshalGoValues(results, host.table)
	if err != nil {
		return nil, fmt.Errorf("marshal operation %d results: %w", operationID, err)
	}
	return wire.EncodeResponse(wire.Response{Values: values})
}

func (host *Host) DropResource(handle uint64) error {
	return host.table.Drop(resources.Handle(handle))
}

func (host *Host) BindCallbackInvoker(invoker native.CallbackInvoker) error {
	if invoker == nil {
		return errors.New("wasm callback invoker is required")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.callbackRoot != nil {
		return errors.New("wasm callback invoker is already bound")
	}
	host.callbackRoot = invoker
	return nil
}

// ReplaceCallbackInvoker installs the serialized top-level callback entry
// after the native runtime has completed its bootstrap binding.
func (host *Host) ReplaceCallbackInvoker(invoker native.CallbackInvoker) error {
	if invoker == nil {
		return errors.New("wasm callback invoker is required")
	}
	host.mu.Lock()
	host.callbackRoot = invoker
	host.mu.Unlock()
	return nil
}

func (host *Host) RegisterCallback(
	callbackTypeID uint32,
	guestID uint64,
) (uint64, error) {
	if callbackTypeID == 0 || int(callbackTypeID) > len(api.GeneratedCallbacks) {
		return 0, fmt.Errorf("unknown generated callback type %d", callbackTypeID)
	}
	host.mu.RLock()
	invoker := host.callbackRoot
	host.mu.RUnlock()
	if invoker == nil {
		return 0, errors.New("wasm callback runtime is not bound")
	}
	callback := api.GeneratedCallbacks[callbackTypeID-1]
	handle, err := host.table.Insert(
		dispatch.GuestCallback{
			TypeID: callbackTypeID, GuestID: guestID,
			Invoker: callbackRouter{host: host},
		},
		callback.Identity,
		resources.LifetimePlugin,
		nil,
	)
	return uint64(handle), err
}

type callbackRouter struct {
	host *Host
}

func (router callbackRouter) InvokeCallback(
	callbackTypeID uint32,
	guestID uint64,
	input []byte,
) ([]byte, error) {
	router.host.mu.RLock()
	invoker := router.host.callbackActive
	if invoker == nil {
		invoker = router.host.callbackRoot
	}
	router.host.mu.RUnlock()
	if invoker == nil {
		return nil, errors.New("wasm callback runtime is not bound")
	}
	output, err := invoker.InvokeCallback(callbackTypeID, guestID, input)
	if !errors.Is(err, native.ErrWrongReentryThread) {
		return output, err
	}
	router.host.mu.RLock()
	root := router.host.callbackRoot
	router.host.mu.RUnlock()
	if root == nil || root == invoker {
		return nil, err
	}
	return root.InvokeCallback(callbackTypeID, guestID, input)
}

func (host *Host) ContextCancelled(contextHandle uint64) (bool, error) {
	ctx, err := resources.ResolveAs[context.Context](
		host.table,
		resources.Handle(contextHandle),
		contextTypeIdentity,
	)
	if err != nil {
		return false, err
	}
	return ctx.Err() != nil, nil
}

func (host *Host) Close() error {
	if host == nil || host.dispatch == nil {
		return nil
	}
	host.StopRegistrations()
	return host.dispatch.Close()
}

// StopRegistrations prevents new external callbacks and removes timers,
// commands, and event subscriptions without invalidating resource handles
// needed while the component store itself is being dropped.
func (host *Host) StopRegistrations() {
	if host == nil {
		return
	}
	host.stopOnce.Do(func() {
		host.mu.Lock()
		host.closed = true
		subscriptions := host.subscriptions
		host.subscriptions = nil
		timers := make([]*ownedTimer, 0, len(host.timers))
		for timer := range host.timers {
			timers = append(timers, timer)
		}
		host.timers = nil
		host.mu.Unlock()
		for _, timer := range timers {
			timer.stop()
		}
		for index := len(subscriptions) - 1; index >= 0; index-- {
			subscriptions[index]()
		}
	})
}

func (host *Host) invokeExtension(
	_ context.Context,
	operation dispatch.Operation,
	arguments []any,
) ([]any, error) {
	switch {
	case strings.HasSuffix(operation.Identity, "#wasm-subscribe"):
		return host.subscribeEvent(operation, arguments)
	case strings.HasSuffix(operation.Identity, "#wasm-register-command"):
		return host.registerCommand(operation, arguments)
	case strings.HasSuffix(operation.Identity, "#wasm-after"):
		return host.scheduleTimer(operation, arguments, false)
	case strings.HasSuffix(operation.Identity, "#wasm-every"):
		return host.scheduleTimer(operation, arguments, true)
	case strings.HasSuffix(operation.Identity, "#wasm-context-cancelled"):
		ctx, err := extensionContext(operation, arguments)
		if err != nil {
			return nil, err
		}
		return []any{ctx.Err() != nil}, nil
	case strings.HasSuffix(operation.Identity, "#wasm-context-deadline"):
		ctx, err := extensionContext(operation, arguments)
		if err != nil {
			return nil, err
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			return []any{int64(0), false}, nil
		}
		return []any{deadline.UnixNano(), true}, nil
	case strings.HasSuffix(operation.Identity, "#wasm-context-error"):
		ctx, err := extensionContext(operation, arguments)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return []any{err.Error()}, nil
		}
		return []any{""}, nil
	case strings.HasSuffix(operation.Identity, "#wasm-log"):
		return host.log(operation, arguments)
	default:
		return nil, fmt.Errorf("unknown wasm runtime extension %s", operation.Identity)
	}
}

func extensionContext(
	operation dispatch.Operation,
	arguments []any,
) (context.Context, error) {
	if len(arguments) != 1 {
		return nil, fmt.Errorf(
			"%s: expected one context argument, got %d",
			operation.Identity,
			len(arguments),
		)
	}
	ctx, ok := arguments[0].(context.Context)
	if !ok || ctx == nil {
		return nil, fmt.Errorf(
			"%s: context has type %T",
			operation.Identity,
			arguments[0],
		)
	}
	return ctx, nil
}

func (host *Host) log(
	operation dispatch.Operation,
	arguments []any,
) ([]any, error) {
	if len(arguments) != 4 {
		return nil, host.dispatch.ArgumentCount(operation, len(arguments), 4)
	}
	ctx, ok := arguments[0].(context.Context)
	if !ok || ctx == nil {
		return nil, fmt.Errorf("log context has type %T", arguments[0])
	}
	level, ok := arguments[1].(int64)
	if !ok {
		return nil, fmt.Errorf("log level has type %T", arguments[1])
	}
	if level < 0 || level > int64(^uint(0)>>1) {
		return nil, fmt.Errorf("log level %d is invalid", level)
	}
	message, ok := arguments[2].(string)
	if !ok {
		return nil, fmt.Errorf("log message has type %T", arguments[2])
	}
	fields, ok := arguments[3].([]string)
	if !ok && arguments[3] != nil {
		return nil, fmt.Errorf("log fields have type %T", arguments[3])
	}
	if len(fields)%2 != 0 {
		return nil, errors.New("log fields must contain key/value pairs")
	}
	values := make([]any, len(fields))
	for index := range fields {
		values[index] = fields[index]
	}
	logr.FromContextOrDiscard(ctx).
		WithName("wasm").
		WithValues("plugin", host.plugin).
		V(int(level)).
		Info(message, values...)
	return nil, nil
}

func (host *Host) subscribeEvent(
	operation dispatch.Operation,
	arguments []any,
) ([]any, error) {
	if len(arguments) != 2 {
		return nil, host.dispatch.ArgumentCount(operation, len(arguments), 2)
	}
	priority, ok := arguments[0].(int)
	if !ok {
		return nil, fmt.Errorf("event subscription priority has type %T", arguments[0])
	}
	handler := reflect.ValueOf(arguments[1])
	if !handler.IsValid() || handler.Kind() != reflect.Func ||
		handler.Type().NumIn() != 1 ||
		handler.Type().NumOut() != 1 ||
		!handler.Type().Out(0).Implements(reflect.TypeFor[error]()) {
		return nil, fmt.Errorf("event subscription handler has invalid type %T", arguments[1])
	}
	eventType := handler.Type().In(0)
	if eventType.Kind() != reflect.Pointer {
		return nil, fmt.Errorf("event subscription requires a pointer event, got %s", eventType)
	}
	var once sync.Once
	unsubscribe := host.eventManager.Subscribe(
		reflect.Zero(eventType).Interface(),
		priority,
		func(fired event.Event) {
			transaction, commit, ok := eventTransaction(fired, eventType)
			if !ok {
				return
			}
			returned := handler.Call([]reflect.Value{transaction})
			if returned[0].IsNil() {
				commit()
			}
		},
	)
	trackedUnsubscribe := func() {
		once.Do(unsubscribe)
	}
	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		trackedUnsubscribe()
		return nil, errors.New("wasm plugin host is closed")
	}
	host.subscriptions = append(host.subscriptions, trackedUnsubscribe)
	host.mu.Unlock()
	return []any{trackedUnsubscribe}, nil
}

func (host *Host) registerCommand(
	operation dispatch.Operation,
	arguments []any,
) ([]any, error) {
	if len(arguments) != 3 {
		return nil, host.dispatch.ArgumentCount(operation, len(arguments), 3)
	}
	name, ok := arguments[0].(string)
	if !ok {
		return nil, fmt.Errorf("command name has type %T", arguments[0])
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return nil, fmt.Errorf("command name %q is invalid", name)
	}
	aliases, ok := arguments[1].([]string)
	if !ok && arguments[1] != nil {
		return nil, fmt.Errorf("command aliases have type %T", arguments[1])
	}
	normalized := make([]string, len(aliases))
	seen := map[string]struct{}{name: {}}
	for index, alias := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" || strings.ContainsAny(alias, " \t\r\n") {
			return nil, fmt.Errorf("command alias %q is invalid", alias)
		}
		if _, duplicate := seen[alias]; duplicate {
			return nil, fmt.Errorf("duplicate command name or alias %q", alias)
		}
		seen[alias] = struct{}{}
		normalized[index] = alias
	}
	execute, ok := arguments[2].(func(*command.Context) error)
	if !ok {
		return nil, fmt.Errorf("command callback has type %T", arguments[2])
	}

	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		return nil, errors.New("wasm plugin host is closed")
	}
	for candidate := range seen {
		if host.commandManager.Has(candidate) {
			host.mu.Unlock()
			return nil, fmt.Errorf("command %q is already registered", candidate)
		}
	}
	host.commandManager.RegisterWithAliases(
		brigodier.Literal(name).Executes(command.Command(execute)),
		normalized...,
	)
	var once sync.Once
	unregister := func() {
		once.Do(func() {
			host.mu.Lock()
			host.commandManager.Root.RemoveChild(append([]string{name}, normalized...)...)
			host.mu.Unlock()
		})
	}
	host.subscriptions = append(host.subscriptions, unregister)
	host.mu.Unlock()
	return []any{unregister}, nil
}

func (host *Host) scheduleTimer(
	operation dispatch.Operation,
	arguments []any,
	recurring bool,
) ([]any, error) {
	if len(arguments) != 2 {
		return nil, host.dispatch.ArgumentCount(operation, len(arguments), 2)
	}
	nanoseconds, ok := arguments[0].(int64)
	if !ok {
		return nil, fmt.Errorf("timer duration has type %T", arguments[0])
	}
	duration := time.Duration(nanoseconds)
	if duration <= 0 {
		return nil, errors.New("timer duration must be greater than zero")
	}
	handler, ok := arguments[1].(func() error)
	if !ok {
		return nil, fmt.Errorf("timer callback has type %T", arguments[1])
	}
	timer := &ownedTimer{
		host: host, duration: duration, recurring: recurring, handler: handler,
	}
	host.mu.Lock()
	if host.closed {
		host.mu.Unlock()
		return nil, errors.New("wasm plugin host is closed")
	}
	if uint32(len(host.timers)) >= host.timerLimit {
		host.mu.Unlock()
		return nil, fmt.Errorf("wasm plugin timer limit %d reached", host.timerLimit)
	}
	host.timers[timer] = struct{}{}
	host.mu.Unlock()
	timer.start()
	return []any{func() { timer.cancel() }}, nil
}

func eventTransaction(
	fired event.Event,
	expected reflect.Type,
) (transaction reflect.Value, commit func(), ok bool) {
	original := reflect.ValueOf(fired)
	if !original.IsValid() || !original.Type().AssignableTo(expected) ||
		original.Kind() != reflect.Pointer || original.IsNil() {
		return reflect.Value{}, nil, false
	}
	copy := reflect.New(original.Elem().Type())
	copy.Elem().Set(original.Elem())
	return copy, func() {
		original.Elem().Set(copy.Elem())
	}, true
}

type ownedTimer struct {
	mu        sync.Mutex
	host      *Host
	timer     *time.Timer
	duration  time.Duration
	handler   func() error
	recurring bool
	stopped   bool
}

func (timer *ownedTimer) start() {
	timer.mu.Lock()
	if !timer.stopped {
		timer.timer = time.AfterFunc(timer.duration, timer.fire)
	}
	timer.mu.Unlock()
}

func (timer *ownedTimer) fire() {
	if err := timer.handler(); err != nil || !timer.recurring {
		timer.cancel()
		return
	}
	timer.start()
}

func (timer *ownedTimer) cancel() {
	timer.stop()
	timer.host.mu.Lock()
	if timer.host.timers != nil {
		delete(timer.host.timers, timer)
	}
	timer.host.mu.Unlock()
}

func (timer *ownedTimer) stop() {
	timer.mu.Lock()
	timer.stopped = true
	if timer.timer != nil {
		timer.timer.Stop()
	}
	timer.mu.Unlock()
}

func encodeDispatchError(operationID uint32, err error) ([]byte, error) {
	operation := generatedOperationIdentity(operationID)
	kind := "host-error"
	var dispatchError *dispatch.Error
	if errors.As(err, &dispatchError) {
		kind = string(dispatchError.Kind)
		if dispatchError.Operation.Identity != "" {
			operation = dispatchError.Operation.Identity
		}
	}
	return wire.EncodeResponse(wire.Response{Error: &wire.GateError{
		Kind: kind, Message: err.Error(), Operation: operation,
	}})
}

func generatedOperationIdentity(id uint32) string {
	if id == 0 || int(id) > len(api.GeneratedOperations) {
		return fmt.Sprintf("operation-%d", id)
	}
	return api.GeneratedOperations[id-1].Identity
}

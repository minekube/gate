// Package gatehost adapts the generated Gate API dispatch table to the native
// Wasmtime host callback.
package gatehost

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.minekube.com/gate/internal/wasm/api"
	"go.minekube.com/gate/internal/wasm/runtime/dispatch"
	"go.minekube.com/gate/internal/wasm/runtime/native"
	"go.minekube.com/gate/internal/wasm/runtime/resources"
	"go.minekube.com/gate/internal/wasm/runtime/wire"
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
		context:  ctx,
		table:    table,
		dispatch: dispatch.NewHost(table),
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
	return host, nil
}

func (host *Host) ContextHandle() uint64 {
	return uint64(host.contextHandle)
}

func (host *Host) ProxyHandle() uint64 {
	return uint64(host.proxyHandle)
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
	return invoker.InvokeCallback(callbackTypeID, guestID, input)
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
	return host.dispatch.Close()
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

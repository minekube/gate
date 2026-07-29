// Package gatehost adapts the generated Gate API dispatch table to the native
// Wasmtime host callback.
package gatehost

import (
	"context"
	"errors"
	"fmt"

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
	context       context.Context
	table         *resources.Table
	dispatch      *dispatch.Host
	contextHandle resources.Handle
	proxyHandle   resources.Handle
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
	return host, nil
}

func (host *Host) ContextHandle() uint64 {
	return uint64(host.contextHandle)
}

func (host *Host) ProxyHandle() uint64 {
	return uint64(host.proxyHandle)
}

func (host *Host) Invoke(operationID uint32, input []byte) ([]byte, error) {
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

// Transform and EmitNested remain only to satisfy the feasibility runtime
// interface while its obsolete entrypoints are removed.
func (host *Host) Transform(uint64, native.Sample) (native.Sample, error) {
	return native.Sample{}, errors.New("spike transform callback is unavailable")
}

func (host *Host) EmitNested(
	native.Reentry,
	uint64,
	string,
) (string, error) {
	return "", errors.New("spike nested callback is unavailable")
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
